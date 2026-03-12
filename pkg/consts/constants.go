package consts

const (
	// Phases
	PhaseActive   = "Active"
	PhasePending  = "Pending"
	PhaseFailed   = "Failed"
	PhaseInactive = "Inactive"

	// Condition Types
	ConditionTypeReady               = "Ready"
	ConditionTypeDNSReady            = "DNSReady"
	ConditionTypeAdoptedRegionsValid = "AdoptedRegionsValid"

	// Condition Reasons
	ReasonReconciliationSucceeded      = "ReconciliationSucceeded"
	ReasonValidationFailed             = "ValidationFailed"
	ReasonSingletonViolation           = "SingletonViolation"
	ReasonInvalidSpec                  = "InvalidSpec"
	ReasonNoAdoptedRegions             = "NoAdoptedRegions"
	ReasonAdoptedRegionNotFound        = "AdoptedRegionNotFound"
	ReasonAllAdoptedRegionsValid       = "AllAdoptedRegionsValid"
	ReasonPolicyInactive               = "PolicyInactive"
	ReasonClusterIdentityNotAvailable  = "ClusterIdentityNotAvailable"
	ReasonDNSConfigurationNotAvailable = "DNSConfigurationNotAvailable" // Unified
	ReasonDNSPolicyNotFound            = "DNSPolicyNotFound"
	ReasonDNSPolicyInactive            = "DNSPolicyInactive"
	ReasonGatewayNotFound              = "GatewayNotFound"
	ReasonNoServiceRoutes              = "NoServiceRoutes"
	ReasonDNSEndpointGenerationFailed  = "DNSEndpointGenerationFailed"
	ReasonIstioGatewayGenerationFailed = "IstioGatewayGenerationFailed"
	ReasonDNSEndpointsCreated          = "DNSEndpointsCreated"
	ReasonLoadBalancerIPPending        = "LoadBalancerIPPending"
	ReasonDNSNotReady                  = "DNSNotReady"
)
