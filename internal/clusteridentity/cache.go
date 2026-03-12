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
	"sync"
)

// ClusterIdentity holds the cluster's regional identity information
type ClusterIdentity struct {
	Region            string
	Cluster           string
	Domain            string
	EnvironmentLetter string
	AdoptsRegions     []string
}

var (
	cache     *ClusterIdentity
	cacheLock sync.RWMutex
)

// Set updates the cluster identity cache
func Set(identity *ClusterIdentity) {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cache = identity
}

// Get retrieves the current cluster identity from cache
func Get() *ClusterIdentity {
	cacheLock.RLock()
	defer cacheLock.RUnlock()
	if cache == nil {
		return nil
	}
	// Return a copy to prevent external modification
	copy := &ClusterIdentity{
		Region:            cache.Region,
		Cluster:           cache.Cluster,
		Domain:            cache.Domain,
		EnvironmentLetter: cache.EnvironmentLetter,
	}
	if len(cache.AdoptsRegions) > 0 {
		copy.AdoptsRegions = make([]string, len(cache.AdoptsRegions))
		for i, v := range cache.AdoptsRegions {
			copy.AdoptsRegions[i] = v
		}
	}
	return copy
}

// Clear removes the cluster identity from cache
func Clear() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cache = nil
}
