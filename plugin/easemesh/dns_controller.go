package easemesh

import (
	"context"
	"crypto/tls"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/megaease/easegress/pkg/object/meshcontroller/spec"
	"go.etcd.io/etcd/api/v3/mvccpb"
	etcdcv3 "go.etcd.io/etcd/client/v3"
	"gopkg.in/yaml.v2"
)

const (
	refreshInterval = 5 * time.Second
	servicePrefix   = "/mesh/service-spec/"
)

// EasegressClient is the client of the easemesh control plane
type dnsController interface {
	ServiceList() []*Service
	ServiceByName(string) []*Service
}

type easegressClient struct {
	clientConfig *etcdcv3.Config
	clientMutex  sync.RWMutex
	client       *etcdcv3.Client

	servicesMutex sync.RWMutex
	services      []*Service
	servicesMap   map[string]*Service

	done chan struct{}
}

func newDNSController(endpoints []string, cc *tls.Config, username, password string) (dnsController, error) {
	// FIXME: Query endpoint from service name easemesh-controlplane-svc.

	etcdCfg := etcdcv3.Config{
		Endpoints:            endpoints,
		TLS:                  cc,
		AutoSyncInterval:     0,
		DialTimeout:          10 * time.Second,
		DialKeepAliveTime:    1 * time.Minute,
		DialKeepAliveTimeout: 1 * time.Minute,
	}
	if username != "" && password != "" {
		etcdCfg.Username = username
		etcdCfg.Password = password
	}

	client := &easegressClient{
		clientConfig: &etcdCfg,
		done:         make(chan struct{}),
	}

	go client.refreshServices()

	return client, nil

}

func (e *easegressClient) getClient() (*etcdcv3.Client, error) {
	e.clientMutex.RLock()
	if e.client != nil {
		client := e.client
		e.clientMutex.RUnlock()
		return client, nil
	}
	e.clientMutex.RUnlock()

	return e.buildClient()
}

func (e *easegressClient) buildClient() (*etcdcv3.Client, error) {
	e.clientMutex.Lock()
	defer e.clientMutex.Unlock()

	if e.client != nil {
		return e.client, nil
	}

	log.Infof("etcd config: %+v", *e.clientConfig)
	client, err := etcdcv3.New(*e.clientConfig)
	if err != nil {
		return nil, err
	}

	e.client = client

	return client, nil
}

func (e *easegressClient) updateServices(services []*Service, servicesMap map[string]*Service) {
	e.servicesMutex.Lock()
	oldServices := e.services
	e.services, e.servicesMap = services, servicesMap
	e.servicesMutex.Unlock()

	oldServiceNames := []string{}
	for _, service := range oldServices {
		oldServiceNames = append(oldServiceNames, service.Name)
	}
	sort.Strings(oldServiceNames)

	serviceNames := []string{}
	for _, service := range services {
		serviceNames = append(serviceNames, service.Name)
	}
	sort.Strings(serviceNames)

	if !reflect.DeepEqual(oldServiceNames, serviceNames) {
		log.Infof("update services from %v to: %v", oldServiceNames, serviceNames)
	}
}

func (e *easegressClient) ServiceList() []*Service {
	e.servicesMutex.RLock()
	defer e.servicesMutex.RUnlock()

	services := make([]*Service, len(e.services))
	copy(services, e.services)

	return services
}

func (e *easegressClient) ServiceByName(name string) []*Service {
	e.servicesMutex.RLock()
	defer e.servicesMutex.RUnlock()

	if len(e.servicesMap) != 0 {
		if e.servicesMap[name] != nil {
			return []*Service{e.servicesMap[name]}
		}
	}

	return nil
}

func (e *easegressClient) refreshServices() {
	ctx := context.Background()
	defer func() {
		e.done = nil
	}()

	duration := refreshInterval
	for {
		C := time.After(duration)
		select {
		case <-C:
			start := time.Now()
			services, servicesMap, err := e.fetchServices(ctx)
			if err != nil {
				log.Errorf("fetch services failed: %v", err)
			} else {
				e.updateServices(services, servicesMap)
			}

			now := time.Now()
			elapse := now.Sub(start)
			if elapse > refreshInterval {
				duration = time.Microsecond * 1
			} else {
				duration = refreshInterval - elapse
			}
		case <-e.done:
			return
		}
	}
}

func (e *easegressClient) fetchServices(ctx context.Context) (
	[]*Service, map[string]*Service, error) {

	client, err := e.getClient()
	if err != nil {
		return nil, nil, fmt.Errorf("get etcd client failed %v: %v", e.clientConfig.Endpoints, err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	resp, err := getPrefix(timeoutCtx, client, servicePrefix)
	if err != nil {
		return nil, nil, fmt.Errorf("get prefix %s failed: %s", servicePrefix, err)
	}

	services := []*Service{}
	serviceMap := map[string]*Service{}
	for _, v := range resp {
		serviceSpec := &spec.Service{}
		err := yaml.Unmarshal([]byte(v), serviceSpec)
		if err != nil {
			log.Errorf("BUG: unmarshal %s to yaml failed: %v", v, err)
			continue
		}
		service := &Service{
			Name:       serviceSpec.Name,
			Tenant:     serviceSpec.RegisterTenant,
			EgressPort: serviceSpec.Sidecar.EgressPort,
		}
		services = append(services, service)
		serviceMap[serviceSpec.Name] = service

	}
	return services, serviceMap, nil
}

func getPrefix(ctx context.Context, cli *etcdcv3.Client, prefix string) (map[string]string, error) {
	kvs := make(map[string]string)
	rawKVs, err := getRawPrefix(ctx, cli, prefix)
	if err != nil {
		return kvs, err
	}

	for _, kv := range rawKVs {
		kvs[string(kv.Key)] = string(kv.Value)
	}

	return kvs, nil
}

func getRawPrefix(ctx context.Context, cli *etcdcv3.Client, prefix string) (map[string]*mvccpb.KeyValue, error) {
	kvs := make(map[string]*mvccpb.KeyValue)

	resp, err := cli.Get(ctx, prefix, etcdcv3.WithPrefix())
	if err != nil {
		return kvs, err
	}

	for _, kv := range resp.Kvs {
		kvs[string(kv.Key)] = kv
	}

	return kvs, nil
}
