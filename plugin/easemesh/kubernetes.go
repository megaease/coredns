package easemesh

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	k8sClientset *kubernetes.Clientset
)

func init() {
	clientset, err := buildClientset()
	if err != nil {
		log.Errorf("build kubernetes clienset failed: %v", err)
	} else {
		k8sClientset = clientset
	}
}

func buildClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("get config in cluster failed: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("new config failed: %v", err)
	}

	return clientset, nil
}

func getEaseMeshEndpoint() (string, error) {
	if k8sClientset == nil {
		return "", fmt.Errorf("kubernetes clientset is not ready")
	}

	service, err := k8sClientset.CoreV1().Services("easemesh").Get(context.Background(),
		"easemesh-control-plane-service", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get easemesh-controlplane-svc failed: %v", err)
	}

	return fmt.Sprintf("http://%s:2379", service.Spec.ClusterIP), nil
}
