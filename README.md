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

You can run the E2E test as long as a SCION daemon is enabled for both the client and the server (which may even be the same daemon within the same AS).
In practice this means that you can run the E2E test on a SCION enabled endhost or using the [test environment](https://docs.scion.org/en/latest/dev/run.html).
Additionally, add an entry to `/etc/hosts` of the form:
 ```
 <target ISD-AS>,[<IP address use to reach SCION services>] scion.local
 ```
Where the ` <target ISD-AS>` is the ISD-AS number the server is running on and `<IP address use to reach SCION services>` is the local address that your host uses to reach the SCION services, i.e. SCION border router and SCION Control service. 

On a SCION enabled endhost, you can find out this addres by inspecting the `etc/scion/topology.json` file:
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
On the local [development test environment](https://docs.scion.org/en/latest/dev/run.html), one can use `127.0.0.1`.

To run the test issue the following command, indicating the `environment.json` for your setup (used for the server) and 

```bash
SCION_ENV_FILE="/tmp/environment.json" go test \
  -timeout 30s \
  -tags=e2e \
  -v \
  -run .\* github.com/scionproto-contrib/caddy-scion/test \
  -sciond-address 127.0.0.1:30255
```
Additionally, if you do not have your SCION environment configuration on the default path, please specify it setting the `SCION_ENV_FILE` environment variable. Note that for this test we only expect **one** AS to be indicated for the server, the format may be similar to:

```
{
    "ases": {
        "1-ff00:0:112": {
             "<target ISD-AS>": "<SCIOND-IP:SCIOND-PORT>"
        }
    }
}
```

Also modify the `-sciond-address` to point to the SCION Daemon that the client will use.