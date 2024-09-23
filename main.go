package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
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

	node := getNode(client, nodeName)

	removeAllAddrLabels(node)

	setInitialAddrLabels(iface, node)

	updateNode(client, node)

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

func setInitialAddrLabels(iface netlink.Link, node *v1.Node) {

	addrs, err := netlink.AddrList(iface, netlink.FAMILY_V4)
	if err != nil {
		fmt.Println("error getting addresses")
	}

	for _, addr := range addrs {
		key := generateLabelKey(convertAddrToString(addr.IP))
		fmt.Println("adding label", key, "to node", node.Name)
		node.Labels[key] = "present"
	}

}

func removeAllAddrLabels(node *v1.Node) {
	for key := range node.Labels {
		if strings.Contains(key, "node.ip/") {
			fmt.Println("removing label", key, "from node", node.Name)
			delete(node.Labels, key)
		}
	}
}

func convertAddrToString(addr net.IP) string {
	if strings.Contains(addr.String(), ":") {
		fmt.Println("ipv6 not supported, will not convert to hyphenated string")
		return "ipv6-unsupported"
	}
	return strings.ReplaceAll(addr.String(), ".", "-")
}

func generateLabelKey(addr string) string {
	return "node.ip/" + addr
}

func getNode(client *kubernetes.Clientset, nodeName string) *v1.Node {
	node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Println("could not get node", nodeName)
		os.Exit(1)
	}

	return node
}

func addAddrLabel(addr net.IP, client *kubernetes.Clientset, nodeName string) {
	node := getNode(client, nodeName)
	key := generateLabelKey(convertAddrToString(addr))
	node.Labels[key] = "present"
	fmt.Println("adding label", key, "to node", node.Name)
	updateNode(client, node)
}

func removeAddrLabel(addr net.IP, client *kubernetes.Clientset, nodeName string) {
	node := getNode(client, nodeName)
	key := generateLabelKey(convertAddrToString(addr))
	delete(node.Labels, key)
	fmt.Println("removing label", key, "from node", node.Name)
	updateNode(client, node)
}

func updateNode(client *kubernetes.Clientset, node *v1.Node) {
	_, err := client.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		fmt.Println("error pushing node changes", err)
	} else {
		fmt.Println("changes saved")
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
