package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

type NodeDiscovery struct {
	projectID    string
	containerSvc *container.Service
	computeSvc   *compute.Service
	cachedIP     string
	cacheTime    time.Time
	cacheTTL     time.Duration
	mutex        sync.RWMutex
}

func New(projectID string) (*NodeDiscovery, error) {
	ctx := context.Background()

	containerSvc, err := container.NewService(ctx, option.WithScopes(container.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create container service: %w", err)
	}

	computeSvc, err := compute.NewService(ctx, option.WithScopes(compute.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create compute service: %w", err)
	}

	return &NodeDiscovery{
		projectID:    projectID,
		containerSvc: containerSvc,
		computeSvc:   computeSvc,
		cacheTTL:     5 * time.Minute,
	}, nil
}

func (d *NodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	d.mutex.RLock()
	if d.cachedIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		ip := d.cachedIP
		d.mutex.RUnlock()
		return ip, nil
	}
	d.mutex.RUnlock()

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.cachedIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		return d.cachedIP, nil
	}

	ip, err := d.discoverNodeIP(ctx)
	if err != nil {
		return "", err
	}

	d.cachedIP = ip
	d.cacheTime = time.Now()
	return ip, nil
}

func (d *NodeDiscovery) discoverNodeIP(ctx context.Context) (string, error) {
	clusters, err := d.containerSvc.Projects.Locations.Clusters.List(
		fmt.Sprintf("projects/%s/locations/-", d.projectID)).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to list clusters: %w", err)
	}

	if len(clusters.Clusters) == 0 {
		return "", fmt.Errorf("no clusters found in project %s", d.projectID)
	}

	cluster := clusters.Clusters[0]
	zone := cluster.Location
	if cluster.Location == "" {
		zone = cluster.Zone
	}

	instances, err := d.computeSvc.Instances.List(d.projectID, zone).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to list instances in zone %s: %w", zone, err)
	}

	for _, instance := range instances.Items {
		if strings.Contains(instance.Name, cluster.Name) {
			for _, ni := range instance.NetworkInterfaces {
				if ni.NetworkIP != "" {
					return ni.NetworkIP, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no node instances found for cluster %s", cluster.Name)
}
