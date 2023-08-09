export CONTAINERD_SNAPSHOTTER=nix 
export CONTAINERD_ADDRESS=/run/user/1001/containerd/containerd.sock
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/1001/rootlesskit-containerd/child_pid) nerdctl -n k8s.io pull docker.io/library/redis:alpine
nsenter -U --preserve-credentials -m -n -t $(cat /run/user/1001/rootlesskit-containerd/child_pid) ctr -n k8s.io run -t --rm --cgroup "" docker.io/library/redis:alpine redis