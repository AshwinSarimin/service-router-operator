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

package clusteridentity

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha1 "github.com/vecozo/service-router-operator/api/cluster/v1alpha1"
)

// Fetch retrieves cluster identity using a cache-first approach.
// It tries the in-memory cache first for performance, then falls back to
// reading the ClusterIdentity CRD as the authoritative source.
// Returns nil if no ClusterIdentity exists.
func Fetch(ctx context.Context, c client.Client) (*ClusterIdentity, error) {
	// Fast path: try cache first
	identity := Get()
	if identity != nil {
		return identity, nil
	}

	// Fallback: read ClusterIdentity CRD (authoritative source)
	var clusterIdentities clusterv1alpha1.ClusterIdentityList
	if err := c.List(ctx, &clusterIdentities); err != nil {
		return nil, err
	}

	if len(clusterIdentities.Items) == 0 {
		return nil, nil
	}

	// Convert first ClusterIdentity CR to ClusterIdentity
	cr := clusterIdentities.Items[0]
	return &ClusterIdentity{
		Region:            cr.Spec.Region,
		Cluster:           cr.Spec.Cluster,
		Domain:            cr.Spec.Domain,
		EnvironmentLetter: cr.Spec.EnvironmentLetter,
		AdoptsRegions:     cr.Spec.AdoptsRegions,
	}, nil
}
