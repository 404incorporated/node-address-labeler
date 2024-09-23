package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	ifaceName := os.Getenv("INTERFACE")
	if ifaceName == "" {
		fmt.Println("INTERFACE env var is not set")
		os.Exit(1)
	}

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		fmt.Println("NODE_NAME env var is not set")
		os.Exit(1)
	}

	fmt.Println("started")

	iface, err := netlink.LinkByName(ifaceName)
	if err != nil {
		fmt.Println("could not get interface", ifaceName)
		os.Exit(1)
	}

	client := makeKubeClient()

	initalAddrs(iface, nodeName, client)

	addrUpdateChan := make(chan netlink.AddrUpdate)

	netlink.AddrSubscribe(addrUpdateChan, nil)

	go func() {
		for update := range addrUpdateChan {
			if update.LinkIndex == iface.Attrs().Index {
				fmt.Println("addr update", update)
				if update.NewAddr {
					addAddrLabel(update.LinkAddress.IP, client, nodeName)
				} else {
					removeAddrLabel(update.LinkAddress.IP, client, nodeName)
				}
			}
		}
	}()

	select {}

}

// removes all node.ip labels and adds current ones, then pushes the changes
func initalAddrs(iface netlink.Link, nodeName string, client *kubernetes.Clientset) {
	err := nodeUpdate(client, nodeName, func(node *v1.Node) {
		for key := range node.Labels {
			if strings.Contains(key, "node.ip/") {
				fmt.Println("removing label", key, "from node", node.Name)
				delete(node.Labels, key)
			}
		}

		addrs, err := netlink.AddrList(iface, netlink.FAMILY_V4)
		if err != nil {
			fmt.Println("error getting addresses")
		}

		for _, addr := range addrs {
			key := generateLabelKey(addr.IP)
			fmt.Println("adding label", key, "to node", node.Name)
			node.Labels[key] = "present"
		}
	})
	if err != nil {
		fmt.Println(err)
	}
}

func convertAddrToString(addr net.IP) string {
	if strings.Contains(addr.String(), ":") {
		fmt.Println("ipv6 not supported, will not convert to hyphenated string")
		return "ipv6-unsupported"
	}
	return strings.ReplaceAll(addr.String(), ".", "-")
}

func generateLabelKey(addr net.IP) string {
	return "node.ip/" + convertAddrToString(addr)
}

func getNode(ctx context.Context, client *kubernetes.Clientset, nodeName string) *v1.Node {
	node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Println("failed to get node", nodeName)
		fmt.Println(err)
		os.Exit(1)
	}

	return node
}

func addAddrLabel(addr net.IP, client *kubernetes.Clientset, nodeName string) {
	err := nodeUpdate(client, nodeName, func(node *v1.Node) {
		key := generateLabelKey(addr)
		node.Labels[key] = "present"
		fmt.Println("adding label", key, "to node", node.Name)
	})
	if err != nil {
		fmt.Println(err)
	}
}

func removeAddrLabel(addr net.IP, client *kubernetes.Clientset, nodeName string) {
	err := nodeUpdate(client, nodeName, func(node *v1.Node) {
		key := generateLabelKey(addr)
		delete(node.Labels, key)
		fmt.Println("removing label", key, "from node", node.Name)
	})
	if err != nil {
		fmt.Println(err)
	}
}

// Run arbitrary function on v1.Node to apply changes. Retries where there are errors.
func nodeUpdate(client *kubernetes.Clientset, nodeName string, updateFunc func(node *v1.Node)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		node := getNode(ctx, client, nodeName)

		updateFunc(node)

		_, err := client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		if err == nil {
			fmt.Println("updated node successfully")
			return nil
		}

		if errors.IsConflict(err) {
			fmt.Println("version mismatch, trying again")
			continue
		}

		return fmt.Errorf("failed to update node: %v", err)

	}

}

func makeKubeClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("in-cluster config not found, falling back to users kubeconfig")

		kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			fmt.Println("error getting kubeconfig", err)
			os.Exit(1)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset

}
