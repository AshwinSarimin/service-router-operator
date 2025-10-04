# Kubebuilder

## Install kubebuilder

First install Go
```
#Download Go

wget https://go.dev/dl/go1.25.1.linux-amd64.tar.gz -O go.tar.gz

#Remove any previous Go installation
rm -rf /usr/local/go 

#Install Go
sudo tar -C /usr/local -xzf go.tar.gz

#Add /usr/local/go/bin to the PATH environment variable
export PATH=$PATH:/usr/local/go/bin


```

Then install kubebuilder

```
# download kubebuilder and install locally.
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/
```

## Create a Project

Create a directory, and then run the init command inside of it to initialize a new project.

```
cd /mnt/c/git/service-router-operator/
mkdir service-router
cd service-router
kubebuilder init --domain teknologi.nl --repo teknologi.nl/service-router
```

## Create 

## Info

Kubebuilder streamlines the development of Kubernetes controllers by providing a structured framework that integrates with controller runtime and Kubernetes API's. It abstracts much of the repetitive setup, and enabled development with efficient and maintainable extensions of Kubernetes functionality.


In this chapter, I continue exploring the powerful world of Kubernetes customization using Operators — this time taking a step further and implementing a custom Kubernetes Controller with Kubebuilder. Instead of simply reacting to changes in a single custom resource, I aim to create a more interactive, policy-enforcing Controller that actively shapes the behavior of running workloads.

Let’s say you run a multi-tenant cluster where different clients are allowed to launch workloads based on some predefined quotas. In a typical case, these quotas are enforced externally — by a CI/CD system or a platform layer. But what if you could enforce runtime quotas directly inside the cluster, at the Controller level?

As a learning path, I plan to create a custom resource (CR) and a Controller with the following responsibilities:

Monitor Pod creations in a specific namespace.
Validate each Pod’s annotation against a set of known client API keys.
Maintain and update client quotas stored in a shared ConfigMap.
Track quota usage by observing which Pods are actively running.
Periodically reduce a client’s quota based on how many Pods they have running.
Automatically stop all client Pods and block new ones once the quota is exhausted.
This is not a typical “hello world” Operator. It is a small but complete policy engine, built entirely within Kubernetes using native building blocks: Custom Resources, Controllers, and the reconciliation loop — all scaffolded and composed using Kubebuilder.

In this article, I will walk you through the design and implementation of such a Controller step by step, highlighting key patterns and architectural decisions along the way. The result will be a working controller that watches, validates, enforces, and adapts, giving you practical insight into writing custom controllers beyond basic CRUD.

To keep things local and reproducible, a lightweight Kubernetes cluster based on kind is used. It spins up Kubernetes clusters inside Docker containers and is perfect for testing Operators in isolated environments.

Let’s get started with setting up the Operator project using Kubebuilder nice scaffolding feature. First, make sure you have kubebuilder, go, and docker installed on your machine. The assumption is that Go ≥1.20 and a Linux/Mac-based development environment.

More information about Operators can be found in one of my previous article:

Kubernetes Operator. Create the one with Kubebuilder.
One of possible way to customize the Kubernetes cluster is to use Operators. They extend Kubernetes capabilities by…
fenyuk.medium.com

Create a fresh directory for your project and initialize it with kubebuilder:


This command scaffolds a new Kubernetes Operator project with Go modules and a clean directory structure. It uses the domain operator.k8s.yfenyuk.io (you can change it to whatever matches your preferred naming convention), and sets the module path for your Go project.

Next, define the first custom API type — ClientQuota. This resource will represent the client’s identity and the quota they are allowed to consume:


This command generates the boilerplate for:

The API definition (api/v1alpha1/clientquota_types.go)
The controller logic stub (controllers/clientquota_controller.go)
The CRD manifest (config/crd/bases/...)
When prompted:

Say “yes” to generating both the resource and the controller.
This gives us a ready-to-extend reconciliation loop for ClientQuota resources.
At this point, the minimal skeleton Operator is ready. Next step is to build up the behavior we described earlier — validating Pods, tracking quotas, and reacting to Pod state — all via Kubernetes-native APIs and custom logic.

In the next section, I’ll define the schema of the ClientQuota resource and explain how it maps to client identity and consumption limits.

CustomResource keeps the list of Clients who are allowed to run Pods in my Cluster and their details:


Test sample in config/samples/quota_v1alpha1_clientquota.yaml
#6: target namespace is playground;

#7: start set of Clients definitions;

#8..#10: detailed Client description with its Name, secret ApiKey, and how many minutes his Pods can run in my Cluster.

The simple idea that Client buys a certain time for his Pods and receives from me a unique ApiKey which he is obliged to set in annotation to each Pod he wants to run. It is the only possibility for Pod to be run in shared playground Kubernetes namespace. Any Pod, that has no (or invalid) ApiKey will be restricted from running. In addition, if quotaMinutest reaches zero, all Client Pod will be killed. Please remember that it is for educational purposes and basic.

Same structure should be reflected as Goland code for CRD:


Manual editing of api/v1alpha1/clientquota_types.go file is required.

Next step is to generate CustomResourceDefinition YAML file. Luckily, Kubebuilder automates this also with make manifests command and the result YAML can be found in file config/crd/bases/quota.operator.k8s.yfenyuk.io_clientquotas.yaml:


quota.operator.k8s.yfenyuk.io_clientquotas.yaml
#3: CustomResourceDefinition kind;

#11: CRD has name ClientQuota;

#23..#37: already familiar structure of CRD, array of objects, each of it has Name, ApiKey and QuotaMinutes.

From now, we have both CustomResourceDefinition and CustomResource YAMLs. so we can deploy it into Kubernetes cluster to be ready for reconciliation:


The central port of each Controller is Reconsile function. The plan is to call it every minute and delete all illegal Pods and decrease the left quota for allowed Pods.

Get Yuri Fenyuk’s stories in your inbox
Join Medium for free to get updates from this writer.

Enter your email
Subscribe
Open file internal/controller/clientquota_controller.go and extend it. Since code is pretty long and can be found in my repo on github, the plan is to start from main entry Reconsile function:


#5..#8: read CustomResource ClientQuota, which keeps details of allowed Clients;

#10..#15: create(using ClientQuota as source of data) or get existing ConfigMap, which contains the left Quota for each Client (function code is below);

#22: Pod’s annotation key where ApiKey is expected. Each Client knows only its own ApiKey and should keep it in secret;

#23: start loop over found in ‘playground’ namespace Pods;

#24..#32: If for any reason Pod has no ApiKey -> kill it;

#36..#53: If Pod’s ApiKey is not specified in active CustomResource -> kill it;

#55..#64: If no ApiKey is found in ConfigMap with left Quota (shouldn’t be the case if access to it is secured) -> again kill Pod;

#65..#70: Current Pod is from legal Client, but NO Quota left -> again kill Pod;

#72..#74: decrease left Quota on one minute;

#76..#79: store ConfigMap with up-to-date Quota in Kubernetes;

#81: Return a positive reconciliation result and ask it to be called again one minute later.

Function to receive ConfigMap:


#4: try to read ConfigMap;

#8..#24: if ConfigMap is not found, initialize it and fill from passed-in function CustomResource by borrowing each Client Name and initial Quota;

#36..#44: build Golang dictionary with Name to ClientQuota and return it back to the Reconsile function.

These two Golang functions contain the logic to maintain ClientQuota state and to keep Pods in the observed namespace in sync with current quotas. Link to the clientquota_controller.go file is here.

For the first test run, Controller can be launched outside of the Kubernetes cluster with Kubebilder’s command make run to run inside VS Code, and the console output will be something like:

Press enter or click to view image in full size

First Operator run
Reconciliation has started and QuotaMap has been created and stored in Kubernetes. As there are no Pods at all, nothing happens.

Still, the initial QuotaMap has been stored (with data borrowed from CustomResource, created before):


#4: Client with Name team-x has 120 in quota;

#5: Client with Name team-y has 60 in quota.

When first allowed Client creates a Pod:


#6: put Pod in playground namespace;

#8: set correct ApiKey teamy456, which belongs to teamy;

#12..#13: Pod sleeps for one hour as an execution simulation.

The next reconciliation cycle shows some activity:

Press enter or click to view image in full size

Second log for ‘Reconciling ClientQuota…’ is followed by log message ‘ClientSpec’ and ‘Pod API Key found in quota, checking usages…’, which means that Client is allowed to run Pod since the correct ApiKey is specified in Pod’s annotation, this Pod will not be killed, controller just decreased Quota in ConfigMap for this client, which is immediately visible in it:


#5: team-y has quota 59 left.

Sure thing, when Quota becomes 0, the reconciliation logic will kill the Pod.

The final step is to build Controller as Docker image and deploy it inside the Kubernetes Cluster.
There are a few important caveats:

as I am using kind Cluster, look at Using kind remark;
need to add line imagePullPolicy: IfNotPresent into file config/manager/manager.yaml, otherwise Kubernetes will try to find Docker image in public registry, not in kind local
while running inside Cluster, Controlled will need to get/set ConfigMap and get/delete Pods, so RBAC needs to be extended with //+kubebuilder:rbac:groups=”” …, which can be inserted directly into Golang controller file.
Deploy Controller:


which should build Docker image clientquota:latest and deploy it as Kubernetes deployment into the Cluster.

If it went well, there will be new Pod with the following, on my Cluster, logs:


Same reconciliation cycle log can be found in #14..#17.
As before, left Quota in ConfigMap continue to get down every minute, and in less than an hour, Client’s Pod will be deleted. If I run two Pods with an allowed ApiKey , the whole quota will be ‘eaten’ in 30 mins.

There is a design weakness at the moment. If redelpoy same Pod after Quota has reached zero, this Pod continues to run up to one minute, until next reconciliation cycle. I plan to tackle it with Admission Control in the next chapter.

Here is full Kubebuilder project sources.

