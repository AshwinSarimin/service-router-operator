module github.com/yourusername/service-router-operator

go 1.18

require (
    k8s.io/api v0.23.0
    k8s.io/apimachinery v0.23.0
    k8s.io/client-go v0.23.0
    sigs.k8s.io/controller-runtime v0.11.0
    sigs.k8s.io/controller-tools v0.6.0
)

replace (
    k8s.io/api => k8s.io/api v0.23.0
    k8s.io/apimachinery => k8s.io/apimachinery v0.23.0
    k8s.io/client-go => k8s.io/client-go v0.23.0
    sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.11.0
    sigs.k8s.io/controller-tools => sigs.k8s.io/controller-tools v0.6.0
)