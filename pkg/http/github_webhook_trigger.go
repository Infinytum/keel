package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/keel-hq/keel/types"
	"github.com/prometheus/client_golang/prometheus"

	log "github.com/sirupsen/logrus"
)

var newGithubWebhooksCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "Github_webhook_requests_total",
		Help: "How many /v1/webhooks/github requests processed, partitioned by image.",
	},
	[]string{"image"},
)

func init() {
	prometheus.MustRegister(newGithubWebhooksCounter)
}

type githubRegistryPackageWebhook struct {
	Action          string `json:"action"`
	RegistryPackage struct {
		Name           string `json:"name"`
		PackageType    string `json:"package_type"`
		PackageVersion struct {
			Version    string `json:"version"`
			PackageURL string `json:"package_url"`
		} `json:"package_version"`
		UpdatedAt string `json:"updated_at"`
	} `json:"registry_package"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type githubPackageV2Webhook struct {
	Action  string `json:"action"`
	Package struct {
		Id             int    `json:"id"`
		Name           string `json:"name"`
		Namespace      string `json:"namespace"`
		Ecosystem      string `json:"ecosystem"`
		PackageVersion struct {
			Name              string `json:"name"`
			PackageURL        string `json:"package_url"`
			ContainerMetadata struct {
				Tag struct {
					Name   string `json:"name"`
					Digest string `json:"digest"`
				} `json:"tag"`
			} `json:"container_metadata"`
		} `json:"package_version"`
	} `json:"package"`
}

// githubHandler - used to react to github webhooks
func (s *TriggerServer) githubHandler(resp http.ResponseWriter, req *http.Request) {
	// GitHub provides different webhook events for each registry.
	// Github Package uses 'registry_package'
	// Github Container Registry uses 'package_v2'
	// events can be classified as 'X-GitHub-Event' in Request Header.
	hookEvent := req.Header.Get("X-GitHub-Event")

	var imageName, imageTag string

	switch hookEvent {
	case "registry_package":
		payload := new(githubRegistryPackageWebhook)
		if err := json.NewDecoder(req.Body).Decode(payload); err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("trigger.githubHandler: failed to decode request")
			resp.WriteHeader(http.StatusBadRequest)
			return
		}

		if payload.RegistryPackage.PackageType != "CONTAINER" {
			resp.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(resp, "registry package type was not CONTAINER")
			return
		}

		if payload.RegistryPackage.PackageVersion.PackageURL == "" { // tag
			resp.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(resp, "package url cannot be empty")
			return
		}

		imageData := strings.Split(payload.RegistryPackage.PackageVersion.PackageURL, ":")
		imageName = imageData[0]
		imageTag = imageData[1]
	}

	if imageName != "" {
		event := types.Event{}
		event.CreatedAt = time.Now()
		event.TriggerName = "github"
		event.Repository.Name = imageName
		event.Repository.Tag = imageTag

		s.trigger(event)
		resp.WriteHeader(http.StatusOK)
		newGithubWebhooksCounter.With(prometheus.Labels{"image": event.Repository.Name}).Inc()
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}
