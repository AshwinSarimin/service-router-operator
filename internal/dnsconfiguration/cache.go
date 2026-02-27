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
	"sync"
)

// ExternalDNSController defines an ExternalDNS controller configuration
type ExternalDNSController struct {
	Name   string
	Region string
}

// DNSConfiguration holds the cluster's DNS configuration
type DNSConfiguration struct {
	ExternalDNSControllers []ExternalDNSController
}

var (
	cache     *DNSConfiguration
	cacheLock sync.RWMutex
)

// Set updates the DNS configuration cache
func Set(config *DNSConfiguration) {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cache = config
}

// Get retrieves the current DNS configuration from cache
func Get() *DNSConfiguration {
	cacheLock.RLock()
	defer cacheLock.RUnlock()
	if cache == nil {
		return nil
	}
	// Return a copy to prevent external modification
	copy := &DNSConfiguration{}
	if len(cache.ExternalDNSControllers) > 0 {
		copy.ExternalDNSControllers = make([]ExternalDNSController, len(cache.ExternalDNSControllers))
		for i, v := range cache.ExternalDNSControllers {
			copy.ExternalDNSControllers[i] = v
		}
	}
	return copy
}

// Clear removes the DNS configuration from cache
func Clear() {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cache = nil
}
