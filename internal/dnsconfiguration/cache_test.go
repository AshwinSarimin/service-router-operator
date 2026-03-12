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
	"testing"
)

func TestSetAndGet(t *testing.T) {
	Clear()

	config := &DNSConfiguration{
		ExternalDNSControllers: []ExternalDNSController{
			{Name: "external-dns-neu", Region: "neu"},
		},
	}

	Set(config)
	retrieved := Get()

	if retrieved == nil {
		t.Fatal("Get returned nil")
	}

	if len(retrieved.ExternalDNSControllers) != 1 {
		t.Errorf("Expected 1 controller, got %d", len(retrieved.ExternalDNSControllers))
	}
	if retrieved.ExternalDNSControllers[0].Name != "external-dns-neu" {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.ExternalDNSControllers[0].Name, "external-dns-neu")
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

	config := &DNSConfiguration{
		ExternalDNSControllers: []ExternalDNSController{
			{Name: "external-dns-neu", Region: "neu"},
		},
	}

	Set(config)
	retrieved := Get()

	retrieved.ExternalDNSControllers[0].Name = "modified"

	secondRetrieve := Get()
	if secondRetrieve.ExternalDNSControllers[0].Name != "external-dns-neu" {
		t.Errorf("Get should return a copy, but modification affected cache: got %s", secondRetrieve.ExternalDNSControllers[0].Name)
	}
}

func TestClear(t *testing.T) {
	config := &DNSConfiguration{
		ExternalDNSControllers: []ExternalDNSController{
			{Name: "external-dns-neu", Region: "neu"},
		},
	}

	Set(config)
	Clear()

	retrieved := Get()
	if retrieved != nil {
		t.Errorf("Clear should remove config from cache, got %v", retrieved)
	}
}

func TestThreadSafety(t *testing.T) {
	Clear()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			config := &DNSConfiguration{
				ExternalDNSControllers: []ExternalDNSController{
					{Name: "external-dns-neu", Region: "neu"},
				},
			}
			Set(config)
		}()
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Get()
		}()
	}

	wg.Wait()
}
