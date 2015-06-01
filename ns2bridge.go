package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"syscall"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const prefix = "/var/run/netns"

var once sync.Once

func CreateBasePath() {
	err := os.MkdirAll(prefix, 0644)
	if err != nil && !os.IsExist(err) {
		fmt.Printf("%v", err)
	}
}

func CreateNamespaceFile(path string) (err error) {
	var f *os.File

	once.Do(CreateBasePath)
	if f, err = os.Create(path); err == nil {
		f.Close()
	}
	return err

}
func LoopbackUp() error {
	iface, _ := netlink.LinkByName("lo")
	return netlink.LinkSetUp(iface)
}

func main() {
	// create a linux bridge, and bridge device not need find by name
	bridge := &netlink.Bridge{netlink.LinkAttrs{Name: "nsbr0"}}
	netlink.LinkAdd(bridge)
	addr, _ := netlink.ParseAddr("192.168.200.1/24")
	netlink.AddrAdd(bridge, addr)
	netlink.LinkSetUp(bridge)

	//create veth pair
	vethp1 := &netlink.Veth{netlink.LinkAttrs{Name: "veth1"}, "veth2"}
	netlink.LinkAdd(vethp1)
	veth1, _ := netlink.LinkByName("veth1")
	veth2, _ := netlink.LinkByName("veth2")
	netlink.LinkSetMaster(veth2, bridge)

	vethp2 := &netlink.Veth{netlink.LinkAttrs{Name: "veth3"}, "veth4"}
	netlink.LinkAdd(vethp2)
	veth3, _ := netlink.LinkByName("veth3")
	veth4, _ := netlink.LinkByName("veth4")
	netlink.LinkSetMaster(veth4, bridge)

	//create namespace file
	path1 := prefix + "/ns1"
	path2 := prefix + "/ns2"
	CreateNamespaceFile(path1)
	CreateNamespaceFile(path2)

	//create namespace ns1 ns2
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origns, _ := netns.Get()
	defer origns.Close()

	// probably create a new process （Unshare）
	ns1, _ := netns.New()
	defer ns1.Close()

	LoopbackUp()

	procNet := fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), syscall.Gettid())
	if err := syscall.Mount(procNet, path1, "bind", syscall.MS_BIND, ""); err != nil {
		fmt.Printf("error: %v", err)
		return
	}

	ns2, _ := netns.New()
	defer ns2.Close()

	LoopbackUp()

	procNet = fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), syscall.Gettid())
	if err := syscall.Mount(procNet, path2, "bind", syscall.MS_BIND, ""); err != nil {
		fmt.Printf("error: %v", err)
		return
	}

	// add veth to ns,and conif IP
	netns.Set(origns)
	netlink.LinkSetNsFd(veth1, int(ns1))
	netlink.LinkSetNsFd(veth3, int(ns2))

	netns.Set(ns1)
	addr, _ = netlink.ParseAddr("192.168.200.2/24")
	veth1, _ = netlink.LinkByName("veth1")
	netlink.AddrAdd(veth1, addr)
	netlink.LinkSetUp(veth1)

	// add default route in ns1
	gw := net.ParseIP("192.168.200.1")
	defaultroute := &netlink.Route{Gw: gw}
	netlink.RouteAdd(defaultroute)

	netns.Set(ns2)
	addr, _ = netlink.ParseAddr("192.168.200.3/24")
	veth3, _ = netlink.LinkByName("veth3")
	netlink.AddrAdd(veth3, addr)
	netlink.LinkSetUp(veth3)

	netns.Set(origns)
}
