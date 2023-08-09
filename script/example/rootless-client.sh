export CONTAINERD_ADDRESS=/run/user/1001/containerd/containerd.sock 
export CONTAINERD_SNAPSHOTTER=native 
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/1001/rootlesskit-containerd/child_pid) ctr images pull docker.io/library/ubuntu:latest 
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/1001/rootlesskit-containerd/child_pid) ctr run -t --rm --fifo-dir /tmp/foo-fifo --cgroup "" docker.io/library/ubuntu:latest foo 