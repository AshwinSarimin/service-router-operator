/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dnsconfiguration

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
)

// Fetch retrieves DNS configuration using a cache-first approach.
// It tries the in-memory cache first for performance, then falls back to
// reading the DNSConfiguration CRD as the authoritative source.
// Returns nil if no DNSConfiguration exists.
func Fetch(ctx context.Context, c client.Client) (*DNSConfiguration, error) {
	// Fast path: try cache first
	config := Get()
	if config != nil {
		return config, nil
	}

	// Fallback: read DNSConfiguration CRD (authoritative source)
	var dnsConfigs clusterv1alpha1.DNSConfigurationList
	if err := c.List(ctx, &dnsConfigs); err != nil {
		return nil, err
	}

	if len(dnsConfigs.Items) == 0 {
		return nil, nil
	}

	// Convert first DNSConfiguration CR to DNSConfiguration
	cr := dnsConfigs.Items[0]

	config = &DNSConfiguration{
		ExternalDNSControllers: make([]ExternalDNSController, len(cr.Spec.ExternalDNSControllers)),
	}

	for i, c := range cr.Spec.ExternalDNSControllers {
		config.ExternalDNSControllers[i] = ExternalDNSController{
			Name:   c.Name,
			Region: c.Region,
		}
	}

	return config, nil
}
