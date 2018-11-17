# birdcage

**birdcage** automaticaly creates and maintain deployments copies (ie. canaries) from existing deployments.

WARNING: this is an experimental project.

It supports applying arbitrary transformation from the model deployment to the new/generated deployment, for instance to:
* Change replicas number
* Change labels and selectors (so you canary is served from a distinct service)
* Change configmap name to configure your canary differently
* Override image name
* ...

birdcage is a Kubernetes operator, configured by custom resources like that one:

```yaml
apiVersion: datadoghq.datadoghq.com/v1alpha1
kind: Birdcage
metadata:
  labels:
    app: birdcage-example
  name: birdcage-example
spec:

  # source is the original deployment we're watching
  sourceObject:
    name: mydeploy
    namespace: default

  # target is our canary (created from sourceObject deployment patched by luaCode)
  targetObject:
    name: mydeploy-canary
    namespace: default

    # luaCode describe what we want to change in the canary/target deployment
    luaCode: |
        function patch(d)
          -- We only want one canary pod
          d.spec.replicas = 1

          -- Change canary's "app" label to "mydeploy-canary"
          d.metadata.labels.app = d.metadata.name
          d.spec.selector.matchLabels.app = d.metadata.name
          d.spec.template.metadata.labels.app = d.metadata.name

          return d
        end
```
