# birdcage

**birdcage** automaticaly creates and maintain "canary" deployments from existing deployments.

It supports applying arbitrary transformation over the new/generated deployment, for instance to:
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
    app: birdcage-sample
  name: birdcage-sample
spec:

  # source is the original deployment we're watching
  sourceObject:
    name: mydeploy
    namespace: default

  # target is our canary (created from sourceObject deployment patched by luaCode)
  targetObject:
    name: mydeploy-canary
    namespace: default

    # luaCode allows us to adapt "source" deployment attribute before using it as a canary ("target") deployment 
    luaCode: |
        function patch(d)
          -- We only want one canary pod
          d.spec.replicas = 1

          -- Change canary's "app" label to "mydeploy-canary" (ie. to keep it out of service's selectors)
          d.metadata.labels.app = d.metadata.name
          d.spec.selector.matchLabels.app = d.metadata.name
          d.spec.template.metadata.labels.app = d.metadata.name

          return d
        end
```
