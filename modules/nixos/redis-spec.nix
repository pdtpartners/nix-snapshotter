{ pkgs, k8sResources, ... }: {
  # Provide an example kubernetes config for redis using a nix-snapshotter
  # image to `kubectl apply -f /etc/kubernetes/redis/`.
  environment.etc."kubernetes/redis/pod.json".source =
    pkgs.writeText
      "redis-pod.json" 
      (builtins.toJSON k8sResources.redisPod);

  environment.etc."kubernetes/redis/service.json".source =
    pkgs.writeText
      "redis-service.json"
      (builtins.toJSON k8sResources.redisService);
}
