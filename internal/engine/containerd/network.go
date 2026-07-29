package containerd

import (
	"context"
	"fmt"

	gocni "github.com/containerd/go-cni"
)

// network wraps CNI to attach and detach containers from FastShip's
// bridge network.
//
// CNI (Container Network Interface) is the standard for wiring containers
// into networks. FastShip does not reinvent bridge creation, IP
// assignment, or namespace plumbing — the CNI plugins installed at
// /opt/cni/bin do that. This type just drives them: "plug this container
// in", "unplug it".
type network struct {
	cni gocni.CNI
}

// newNetwork initializes CNI using the config at /etc/cni/net.d.
//
// It loads the fastship.conflist we wrote — the bridge network definition.
// go-cni reads that config and knows which plugins to invoke.
func newNetwork() (*network, error) {
	cni, err := gocni.New(
		// Where the plugin binaries live.
		gocni.WithPluginDir([]string{"/opt/cni/bin"}),
		// Load the network config named "fastship" from /etc/cni/net.d.
		gocni.WithConfListFile("/etc/cni/net.d/10-fastship.conflist"),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing CNI: %w", err)
	}
	return &network{cni: cni}, nil
}

// attach plugs a container into the fastship network and returns the IP
// it was assigned.
//
// netnsPath is the path to the container's network namespace — the
// isolated network view CNI will wire into the bridge. id uniquely
// identifies this attachment so it can be torn down later.
func (n *network) attach(ctx context.Context, id, netnsPath string) (string, error) {
	result, err := n.cni.Setup(ctx, id, netnsPath)
	if err != nil {
		return "", fmt.Errorf("attaching %s to network: %w", id, err)
	}

	// Pull the assigned IP out of the CNI result. A container may have
	// several interfaces; we want the first real IP.
	for _, iface := range result.Interfaces {
		for _, ipConf := range iface.IPConfigs {
			return ipConf.IP.String(), nil
		}
	}
	return "", fmt.Errorf("no IP assigned to %s", id)
}

// detach unplugs a container from the network. Called on stop so the IP
// is released and the veth pair is cleaned up.
func (n *network) detach(ctx context.Context, id, netnsPath string) error {
	if err := n.cni.Remove(ctx, id, netnsPath); err != nil {
		return fmt.Errorf("detaching %s from network: %w", id, err)
	}
	return nil
}
