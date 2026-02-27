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
	"testing"
)

func TestSetAndGet(t *testing.T) {
	Clear()

	identity := &ClusterIdentity{
		Region:            "neu",
		Cluster:           "aks01",
		Domain:            "example.com",
		EnvironmentLetter: "d",
	}

	Set(identity)
	retrieved := Get()

	if retrieved == nil {
		t.Fatal("Get returned nil")
	}

	if retrieved.Region != identity.Region {
		t.Errorf("Region mismatch: got %s, want %s", retrieved.Region, identity.Region)
	}
	if retrieved.Cluster != identity.Cluster {
		t.Errorf("Cluster mismatch: got %s, want %s", retrieved.Cluster, identity.Cluster)
	}
	if retrieved.Domain != identity.Domain {
		t.Errorf("Domain mismatch: got %s, want %s", retrieved.Domain, identity.Domain)
	}
	if retrieved.EnvironmentLetter != identity.EnvironmentLetter {
		t.Errorf("EnvironmentLetter mismatch: got %s, want %s", retrieved.EnvironmentLetter, identity.EnvironmentLetter)
	}
}

func TestGetReturnsNilWhenNotSet(t *testing.T) {
	Clear()

	retrieved := Get()
	if retrieved != nil {
		t.Errorf("Get should return nil when not set, got %v", retrieved)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	Clear()

	identity := &ClusterIdentity{
		Region:            "neu",
		Cluster:           "aks01",
		Domain:            "example.com",
		EnvironmentLetter: "d",
	}

	Set(identity)
	retrieved := Get()

	// Modify the retrieved copy
	retrieved.Region = "modified"

	// Get again and verify original is unchanged
	secondRetrieve := Get()
	if secondRetrieve.Region != "neu" {
		t.Errorf("Get should return a copy, but modification affected cache: got %s", secondRetrieve.Region)
	}
}

func TestClear(t *testing.T) {
	identity := &ClusterIdentity{
		Region:            "neu",
		Cluster:           "aks01",
		Domain:            "example.com",
		EnvironmentLetter: "d",
	}

	Set(identity)
	Clear()

	retrieved := Get()
	if retrieved != nil {
		t.Errorf("Get should return nil after Clear, got %v", retrieved)
	}
}

func TestConcurrentAccess(t *testing.T) {
	Clear()

	identity := &ClusterIdentity{
		Region:            "neu",
		Cluster:           "aks01",
		Domain:            "example.com",
		EnvironmentLetter: "d",
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			Set(identity)
		}()
	}
	wg.Wait()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			retrieved := Get()
			if retrieved == nil {
				t.Error("Get returned nil during concurrent access")
			}
		}()
	}
	wg.Wait()
}
