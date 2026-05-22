# External Reverse Proxy Guide

Route your external proxy to `app:3000`. Keep forwarded headers intact.

## Required forwarded headers

- `Host`
- `X-Forwarded-For`
- `X-Forwarded-Proto` (must be `https` in production)

## Nginx example

```nginx
location / {
  proxy_pass http://app:3000;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header X-Forwarded-Proto $scheme;
}
```

## Caddy example

```caddyfile
space.example.com {
  reverse_proxy app:3000 {
    header_up Host {host}
    header_up X-Forwarded-For {remote_host}
    header_up X-Forwarded-Proto {scheme}
  }
}
```

## Traefik example (dynamic file)

```yaml
http:
  routers:
    space:
      rule: Host(`space.example.com`)
      service: space
      tls: {}
  services:
    space:
      loadBalancer:
        servers:
          - url: http://app:3000
```
