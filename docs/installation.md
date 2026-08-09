# Installation

## Traefik Plugin Catalog

Declare the plugin in Traefik's static configuration:

```yaml
experimental:
  plugins:
    x402:
      moduleName: github.com/wyh0626/traefik-x402
      version: v0.1.2
```

Restart Traefik after adding or changing a plugin version. Traefik loads plugins
during startup.

## File provider

```yaml
http:
  routers:
    paid:
      rule: Host(`api.example.com`) && Path(`/premium`)
      service: api
      middlewares:
        - x402-payment

  middlewares:
    x402-payment:
      plugin:
        x402:
          facilitatorURL: https://x402.org/facilitator
          scheme: exact
          network: "eip155:84532"
          asset: "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
          amount: "1000"
          payTo: "0xYourReceiverAddress"
          allowedMethods:
            - GET
            - HEAD
          description: Premium API
          mimeType: application/json
          extra:
            name: USDC
            version: "2"

  services:
    api:
      loadBalancer:
        servers:
          - url: http://api:8080
```

## Docker labels

The plugin declaration remains in Traefik's static configuration. Attach a
middleware and router using labels on the protected service:

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.paid.rule=Host(`api.example.com`) && Path(`/premium`)
  - traefik.http.routers.paid.middlewares=x402-payment
  - traefik.http.middlewares.x402-payment.plugin.x402.scheme=exact
  - traefik.http.middlewares.x402-payment.plugin.x402.network=eip155:84532
  - traefik.http.middlewares.x402-payment.plugin.x402.asset=0x036CbD53842c5426634e7929541eC2318f3dCF7e
  - traefik.http.middlewares.x402-payment.plugin.x402.amount=1000
  - traefik.http.middlewares.x402-payment.plugin.x402.payTo=0xYourReceiverAddress
  - traefik.http.middlewares.x402-payment.plugin.x402.extra.name=USDC
  - traefik.http.middlewares.x402-payment.plugin.x402.extra.version=2
```

Do not put facilitator credentials in labels that are committed to a repository.

## Kubernetes CRD

After declaring the plugin in Traefik's static configuration, create a
Middleware resource:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: x402-payment
  namespace: default
spec:
  plugin:
    x402:
      facilitatorURL: https://x402.org/facilitator
      scheme: exact
      network: "eip155:84532"
      asset: "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
      amount: "1000"
      payTo: "0xYourReceiverAddress"
      allowedMethods:
        - GET
        - HEAD
      description: Premium API
      mimeType: application/json
      extra:
        name: USDC
        version: "2"
```

Reference it from an IngressRoute middleware list in the normal Traefik manner.

## CORS preflight

Browser `OPTIONS` preflight requests should normally not be charged. Create a
higher-priority `OPTIONS` router without the x402 middleware, or exclude
preflight at the routing layer. The plugin intentionally does not infer which
requests are free.

## Local plugin development

Mount the source at the path matching the module name:

```text
/plugins-local/src/github.com/wyh0626/traefik-x402
```

Then use this static configuration instead of the catalog declaration:

```yaml
experimental:
  localPlugins:
    x402:
      moduleName: github.com/wyh0626/traefik-x402
```
