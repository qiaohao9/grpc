# Encryption

The example for encryption includes two individual examples for TLS and ALTS
encryption mechanism respectively.

## Try it

In each example's subdirectory:

```
go run server/main.go
```

```
go run client/main.go
```

## Explanation

### TLS

TLS is a commonly used cryptographic protocol to provide end-to-end
communication security. In the example, we show how to set up a server
authenticated TLS connection to transmit RPC.

In our `grpc/credentials` package, we provide several convenience methods to
create grpc
[`credentials.TransportCredentials`](https://godoc.org/github.com/qiaohao9/grpc/credentials#TransportCredentials)
base on TLS. Refer to the
[godoc](https://godoc.org/github.com/qiaohao9/grpc/credentials) for details.

In our example, we use the public/private keys created ahead: 
* "server_cert.pem" contains the server certificate (public key). 
* "server_key.pem" contains the server private key. 
* "ca_cert.pem" contains the certificate (certificate authority)
that can verify the server's certificate.

On server side, we provide the paths to "server.pem" and "server.key" to
configure TLS and create the server credential using
[`credentials.NewServerTLSFromFile`](https://godoc.org/github.com/qiaohao9/grpc/credentials#NewServerTLSFromFile).

On client side, we provide the path to the "ca_cert.pem" to configure TLS and create
the client credential using
[`credentials.NewClientTLSFromFile`](https://godoc.org/github.com/qiaohao9/grpc/credentials#NewClientTLSFromFile).
Note that we override the server name with "x.test.example.com", as the server
certificate is valid for *.test.example.com but not localhost. It is solely for
the convenience of making an example.

Once the credentials have been created at both sides, we can start the server
with the just created server credential (by calling
[`grpc.Creds`](https://godoc.org/github.com/qiaohao9/grpc#Creds)) and let client dial
to the server with the created client credential (by calling
[`grpc.WithTransportCredentials`](https://godoc.org/github.com/qiaohao9/grpc#WithTransportCredentials))

And finally we make an RPC call over the created `grpc.ClientConn` to test the secure
connection based upon TLS is successfully up.

### ALTS
NOTE: ALTS currently needs special early access permission on GCP. You can ask 
about the detailed process in https://groups.google.com/forum/#!forum/grpc-io.

ALTS is the Google's Application Layer Transport Security, which supports mutual
authentication and transport encryption. Note that ALTS is currently only
supported on Google Cloud Platform, and therefore you can only run the example
successfully in a GCP environment. In our example, we show how to initiate a
secure connection that is based on ALTS.

Unlike TLS, ALTS makes certificate/key management transparent to user. So it is
easier to set up.

On server side, first call
[`alts.DefaultServerOptions`](https://godoc.org/github.com/qiaohao9/grpc/credentials/alts#DefaultServerOptions)
to get the configuration for alts and then provide the configuration to
[`alts.NewServerCreds`](https://godoc.org/github.com/qiaohao9/grpc/credentials/alts#NewServerCreds)
to create the server credential based upon alts.

On client side, first call
[`alts.DefaultClientOptions`](https://godoc.org/github.com/qiaohao9/grpc/credentials/alts#DefaultClientOptions)
to get the configuration for alts and then provide the configuration to
[`alts.NewClientCreds`](https://godoc.org/github.com/qiaohao9/grpc/credentials/alts#NewClientCreds)
to create the client credential based upon alts.

Next, same as TLS, start the server with the server credential and let client
dial to server with the client credential.

Finally, make an RPC to test the secure connection based upon ALTS is
successfully up.