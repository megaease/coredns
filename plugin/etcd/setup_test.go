// +build etcd

package etcd

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupEtcd(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedPath       string
		expectedEndpoint   []string
		expectedErrContent string // substring from the expected error. Empty for positive cases.
		username           string
		password           string
	}{
		// positive
		{
			`etcd`, false, "skydns", []string{"http://localhost:2379"}, "", "", "",
		},
		{
			`etcd coredns.local test.local {
	endpoint http://localhost:2379 http://localhost:3379 http://localhost:4379

}`, false, "skydns", []string{"http://localhost:2379", "http://localhost:3379", "http://localhost:4379"}, "", "", "",
		},
		{
			`etcd skydns.local {
	endpoint localhost:300
}
`, false, "skydns", []string{"localhost:300"}, "", "", "",
		},
		// negative
		{
			`etcd {
	endpoints localhost:300
}
`, true, "", []string{""}, "unknown property 'endpoints'", "", "",
		},
		// with valid credentials
		{
			`etcd {
			endpoint http://localhost:2379
			credentials username password
		}
			`, false, "skydns", []string{"http://localhost:2379"}, "", "username", "password",
		},
		// with credentials, missing password
		{
			`etcd {
			endpoint http://localhost:2379
			credentials username
		}
			`, true, "skydns", []string{"http://localhost:2379"}, "credentials requires 2 arguments", "username", "",
		},
		// with credentials, missing username and  password
		{
			`etcd {
			endpoint http://localhost:2379
			credentials
		}
			`, true, "skydns", []string{"http://localhost:2379"}, "Wrong argument count", "", "",
		},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		etcd, err := etcdParse(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: Expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: Expected no error but found one for input %s. Error was: %v", i, test.input, err)
				continue
			}

			if !strings.Contains(err.Error(), test.expectedErrContent) {
				t.Errorf("Test %d: Expected error to contain: %v, found error: %v, input: %s", i, test.expectedErrContent, err.Error(), test.input)
				continue
			}
		}

		if !test.shouldErr && etcd.PathPrefix != test.expectedPath {
			t.Errorf("Etcd not correctly set for input %s. Expected: %s, actual: %s", test.input, test.expectedPath, etcd.PathPrefix)
		}
		if !test.shouldErr {
			if len(etcd.endpoints) != len(test.expectedEndpoint) {
				t.Errorf("Etcd not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.expectedEndpoint, etcd.endpoints)
			}
			for i, endpoint := range etcd.endpoints {
				if endpoint != test.expectedEndpoint[i] {
					t.Errorf("Etcd not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.expectedEndpoint, etcd.endpoints)
				}
			}
		}

		if !test.shouldErr {
			if test.username != "" {
				if etcd.Client.Username != test.username {
					t.Errorf("Etcd username not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.username, etcd.Client.Username)
				}
			}
			if test.password != "" {
				if etcd.Client.Password != test.password {
					t.Errorf("Etcd password not correctly set for input %s. Expected: '%+v', actual: '%+v'", test.input, test.password, etcd.Client.Password)
				}
			}
		}
	}
}
