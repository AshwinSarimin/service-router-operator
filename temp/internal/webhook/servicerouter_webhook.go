package webhook

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/kubernetes/pkg/apis/admission"
	"k8s.io/kubernetes/pkg/apis/admission/v1"
	"k8s.io/kubernetes/pkg/admission"
	"k8s.io/kubernetes/pkg/admission/plugin/webhook"
	"k8s.io/kubernetes/pkg/registry/core/service"
	"k8s.io/kubernetes/pkg/registry/core/service/strategy"
	"k8s.io/kubernetes/pkg/util/validation"
)

type ServiceRouterWebhook struct {
	decoder *admission.Decoder
}

func NewServiceRouterWebhook(decoder *admission.Decoder) *ServiceRouterWebhook {
	return &ServiceRouterWebhook{decoder: decoder}
}

func (wh *ServiceRouterWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var admissionReview v1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&admissionReview); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := wh.handleAdmissionReview(admissionReview)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (wh *ServiceRouterWebhook) handleAdmissionReview(review v1.AdmissionReview) *v1.AdmissionResponse {
	var admissionResponse v1.AdmissionResponse
	admissionResponse.UID = review.Request.UID

	switch review.Request.Operation {
	case v1.Create:
		admissionResponse = wh.validateCreate(review.Request)
	case v1.Update:
		admissionResponse = wh.validateUpdate(review.Request)
	case v1.Delete:
		admissionResponse = wh.validateDelete(review.Request)
	default:
		admissionResponse.Allowed = false
		admissionResponse.Result = &metav1.Status{
			Message: "unsupported operation",
		}
	}

	return &admissionResponse
}

func (wh *ServiceRouterWebhook) validateCreate(req *v1.AdmissionRequest) v1.AdmissionResponse {
	// Implement validation logic for create requests
	return v1.AdmissionResponse{Allowed: true}
}

func (wh *ServiceRouterWebhook) validateUpdate(req *v1.AdmissionRequest) v1.AdmissionResponse {
	// Implement validation logic for update requests
	return v1.AdmissionResponse{Allowed: true}
}

func (wh *ServiceRouterWebhook) validateDelete(req *v1.AdmissionRequest) v1.AdmissionResponse {
	// Implement validation logic for delete requests
	return v1.AdmissionResponse{Allowed: true}
}