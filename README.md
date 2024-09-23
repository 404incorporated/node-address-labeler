# Node Address Labeler
The purpose of this project is to automatically add labels to nodes for ip addresses on a specified interface.  
This program works by watching the specified interface and adding/removing labels with the prefix of "node.ip/".  
For example if the IP address 192.168.100.123/32 is added to our interface, then `node.ip/192-168-100-123: "present"` is added to the node.  
When the IP address is removed then the label is removed.  
  
I made this to automatically add IP address labels for use with Cilium Egress Gateway and Kube-vip.

# Running

When running make sure the following env vars are specified.
```
INTERFACE="eth0"
NODE_NAME="node01"
```