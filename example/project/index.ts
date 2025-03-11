import * as pulumi from "@pulumi/pulumi";
import * as docker from "@pulumi/docker";
import * as command from "@pulumi/command";

// Configure Docker provider to use Lima socket
const dockerProvider = new docker.Provider("lima-docker", {
  host: "unix:///Users/cjs/.lima/docker/sock/docker.sock",
});

// Redis container with default network
const redis = new docker.Container("redis", {
  image: "redis:alpine",
  ports: [{
    internal: 6379,
    external: 6379,
  }],
  restart: "unless-stopped",
}, { provider: dockerProvider });

// Run redis-cli to get version (after the container is running)
const redisVersion = new command.local.Command("redis-version", {
  create: "docker exec redis redis-cli info server | grep redis_version | cut -d':' -f2",
  update: "docker exec redis redis-cli info server | grep redis_version | cut -d':' -f2",
}, { dependsOn: [redis] });

// Export the Redis version
export const version = redisVersion.stdout;
