# Caddy-SCION pluggins

[SCION](https://docs.scion.org/en/latest/) plugins for [caddy](https://caddyserver.com/) webserver.

This includes:
- A native SCION listener.
- A wrapper for the [SCION HTTP Proxy](https://github.com/scionproto-contrib/http-proxy) implementation.

## User/admin setup

If you are looking for installing and configuring the SCION-HTTP proxy as an user or network administrator, please refer to the [HTTP Proxy Documentation](https://scion-http-proxy.readthedocs.io/en/latest/index.html).

## Developer setup

If you are looking for setting up a developer environment, you can directly refer to the [Development Setup](https://scion-http-proxy.readthedocs.io/en/latest/dev_setup.html) section.

## E2E tests

Only runnable on a SCION enabled host. Additionally the add an entry to `/etc/hosts` of the form:
 ```
 <local ISD-AS>,[<IP address use to reach SCION services>] scion.local
 ```
Where the ` <local ISD-AS>` is the ISD-AS number the host is running on. (one can verify it in the `topology.json` by default this is located under `/etc/scion`) and `<IP address use to reach SCION services>` is the local address that your host uses to reach the SCION services, i.e. SCION border router and SCION Control service. You can find out this addres by inspecting the `etc/scion/topology.json` file:
```
"control_service": {
    "cs-1": {
      "addr": "<IP address use to reach SCION services>"
    },
  }
```
and then issuing:
```
$ sudo ip route get <IP address use to reach SCION services>
```

If your SCION daemon is not listening on the default address i.e. `127.0.0.1:30255`, provide the actual address as a flag to the test.

```bash
go test \
  -timeout 30s \
  -tags=e2e \
  -v \
  -run .\* github.com/scionproto-contrib/http-proxy/test \
  -sciond-address 127.0.0.1:30255
```
Additionally, if you do not have your SCION environment configuration on the default path, please specify it setting the `SCION_ENV_FILE` environment variable.