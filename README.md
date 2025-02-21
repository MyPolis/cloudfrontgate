# cloudfrontgate - Traefik Plugin
A Traefik middleware plugin that validates incoming requests against Amazon Cloudfront's IP ranges, ensuring services are only accessed through Amazon Cloudfront's proxy.

> Heavily _inspired_ by [sstoner/cloudflaregate](https://github.com/sstoner/cloudflaregate/) plugin.

[![Build Status](https://github.com/portswigger-cloud/cloudfrontgate/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/portswigger-cloud/cloudfrontgate/actions)
[![Go Report](https://goreportcard.com/badge/github.com/portswigger-cloud/cloudfrontgate)](https://goreportcard.com/report/github.com/portswigger-cloud/cloudfrontgate)
[![Go Coverage](https://github.com/portswigger-cloud/cloudfrontgate/wiki/coverage.svg)](https://raw.githack.com/wiki/portswigger-cloud/cloudfrontgate/coverage.html)
[![Latest Release](https://img.shields.io/github/v/release/portswigger-cloud/cloudfrontgate)](https://github.com/portswigger-cloud/cloudfrontgate/releases/latest)


## Features

- Validates that incoming requests originate from Amazon CloudFront's IP ranges
- Automatic periodic updates of Amazon CloudFront IP ranges
- Allow additional IP addresses or CIDR ranges

## Configuration

### Static Configuration

To use this plugin in your Traefik instance, register it in the static configuration:

```yaml
# Static configuration
experimental:
  plugins:
    cloudfrontgate:
      moduleName: github.com/portswigger-cloud/cloudfrontgate
      version: v0.0.4
```


### Dynamic Configuration

Configure the middleware in your dynamic configuration:

```yaml
# Dynamic configuration
http:
  middlewares:
    cloudfront-gate:
      plugin:
        cloudfrontgate:
          # Optional: configure IP ranges refresh interval (default: 24h)
          refreshInterval: "24h"
          # Allow internal traffic
          allowedIPs:
            - "192.168.1.0/24"

  routers:
    my-router:
      rule: Host(`app.example.com`)
      service: my-service
      middlewares:
        - cloudfront-gate
      entryPoints:
        - websecure

  services:
    my-service:
      loadBalancer:
        servers:
          - url: http://internal-service:8080
```

## Configuration Options

| Option           | Type       | Default | Description                                                  |
|------------------|------------|---------|--------------------------------------------------------------|
| `refreshInterval`| string     | `24h`   | Interval for updating CloudFront IP ranges (minimum: 1s)     |
| `allowedIPs`     | []string   | `[]`    | List of additional IP addresses or CIDR ranges to allow      |

### Example Configuration

```yaml
# Static configuration
experimental:
  plugins:
    cloudfrontgate:
      moduleName: github.com/portswigger-cloud/cloudfrontgate
      version: v0.0.3
```

## Security Features

## Development

### Prerequisites
- Go 1.22.0 or later
- Traefik 2.x

### Building
```bash
# Clone the repository
git clone https://github.com/portswigger-cloud/cloudfrontgate
cd cloudfrontgate

# Run tests
make test

# Build
go build ./...
```

### Testing Locally

For local testing, use Traefik's development mode:

```yaml
# Static configuration
experimental:
  localPlugins:
    cloudfrontgate:
      moduleName: github.com/portswigger-cloud/cloudfrontgate
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Thanks to the Traefik team for their plugin system
- Amazon CloudFront for providing their IP ranges publicly

## Support

If you encounter any issues or have questions:
- Open an issue on [GitHub](https://github.com/portswigger-cloud/cloudfrontgate/issues)
- Check existing issues for solutions
