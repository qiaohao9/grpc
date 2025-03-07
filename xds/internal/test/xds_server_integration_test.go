//go:build !386
// +build !386

/*
 *
 * Copyright 2020 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package xds_test contains e2e tests for xDS use.
package xds_test

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/qiaohao9/grpc"
	"github.com/qiaohao9/grpc/codes"
	"github.com/qiaohao9/grpc/credentials/insecure"
	"github.com/qiaohao9/grpc/internal/testutils"
	"github.com/qiaohao9/grpc/status"
	"github.com/qiaohao9/grpc/xds"
	"github.com/qiaohao9/grpc/xds/internal/testutils/e2e"

	v3corepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v3listenerpb "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	v3routepb "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	v3routerpb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	v3httppb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	wrapperspb "github.com/golang/protobuf/ptypes/wrappers"
	xdscreds "github.com/qiaohao9/grpc/credentials/xds"
	testpb "github.com/qiaohao9/grpc/test/grpc_testing"
	xdstestutils "github.com/qiaohao9/grpc/xds/internal/testutils"
)

const (
	// Names of files inside tempdir, for certprovider plugin to watch.
	certFile = "cert.pem"
	keyFile  = "key.pem"
	rootFile = "ca.pem"
)

// setupGRPCServer performs the following:
// - spin up an xDS-enabled gRPC server, configure it with xdsCredentials and
//   register the test service on it
// - create a local TCP listener and start serving on it
//
// Returns the following:
// - local listener on which the xDS-enabled gRPC server is serving on
// - cleanup function to be invoked by the tests when done
func setupGRPCServer(t *testing.T, bootstrapContents []byte) (net.Listener, func()) {
	t.Helper()

	// Configure xDS credentials to be used on the server-side.
	creds, err := xdscreds.NewServerCredentials(xdscreds.ServerOptions{
		FallbackCreds: insecure.NewCredentials(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Initialize an xDS-enabled gRPC server and register the stubServer on it.
	server := xds.NewGRPCServer(grpc.Creds(creds), xds.BootstrapContentsForTesting(bootstrapContents))
	testpb.RegisterTestServiceServer(server, &testService{})

	// Create a local listener and pass it to Serve().
	lis, err := xdstestutils.LocalTCPListener()
	if err != nil {
		t.Fatalf("testutils.LocalTCPListener() failed: %v", err)
	}

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Errorf("Serve() failed: %v", err)
		}
	}()

	return lis, func() {
		server.Stop()
	}
}

func hostPortFromListener(lis net.Listener) (string, uint32, error) {
	host, p, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return "", 0, fmt.Errorf("net.SplitHostPort(%s) failed: %v", lis.Addr().String(), err)
	}
	port, err := strconv.ParseInt(p, 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("strconv.ParseInt(%s, 10, 32) failed: %v", p, err)
	}
	return host, uint32(port), nil
}

// TestServerSideXDS_Fallback is an e2e test which verifies xDS credentials
// fallback functionality.
//
// The following sequence of events happen as part of this test:
// - An xDS-enabled gRPC server is created and xDS credentials are configured.
// - xDS is enabled on the client by the use of the xds:/// scheme, and xDS
//   credentials are configured.
// - Control plane is configured to not send any security configuration to both
//   the client and the server. This results in both of them using the
//   configured fallback credentials (which is insecure creds in this case).
func (s) TestServerSideXDS_Fallback(t *testing.T) {
	managementServer, nodeID, bootstrapContents, resolver, cleanup1 := setupManagementServer(t)
	defer cleanup1()

	lis, cleanup2 := setupGRPCServer(t, bootstrapContents)
	defer cleanup2()

	// Grab the host and port of the server and create client side xDS resources
	// corresponding to it. This contains default resources with no security
	// configuration in the Cluster resources.
	host, port, err := hostPortFromListener(lis)
	if err != nil {
		t.Fatalf("failed to retrieve host and port of server: %v", err)
	}
	const serviceName = "my-service-fallback"
	resources := e2e.DefaultClientResources(e2e.ResourceParams{
		DialTarget: serviceName,
		NodeID:     nodeID,
		Host:       host,
		Port:       port,
		SecLevel:   e2e.SecurityLevelNone,
	})

	// Create an inbound xDS listener resource for the server side that does not
	// contain any security configuration. This should force the server-side
	// xdsCredentials to use fallback.
	inboundLis := e2e.DefaultServerListener(host, port, e2e.SecurityLevelNone)
	resources.Listeners = append(resources.Listeners, inboundLis)

	// Setup the management server with client and server-side resources.
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	if err := managementServer.Update(ctx, resources); err != nil {
		t.Fatal(err)
	}

	// Create client-side xDS credentials with an insecure fallback.
	creds, err := xdscreds.NewClientCredentials(xdscreds.ClientOptions{
		FallbackCreds: insecure.NewCredentials(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a ClientConn with the xds scheme and make a successful RPC.
	cc, err := grpc.DialContext(ctx, fmt.Sprintf("xds:///%s", serviceName), grpc.WithTransportCredentials(creds), grpc.WithResolvers(resolver))
	if err != nil {
		t.Fatalf("failed to dial local test server: %v", err)
	}
	defer cc.Close()

	client := testpb.NewTestServiceClient(cc)
	if _, err := client.EmptyCall(ctx, &testpb.Empty{}, grpc.WaitForReady(true)); err != nil {
		t.Errorf("rpc EmptyCall() failed: %v", err)
	}
}

// TestServerSideXDS_FileWatcherCerts is an e2e test which verifies xDS
// credentials with file watcher certificate provider.
//
// The following sequence of events happen as part of this test:
// - An xDS-enabled gRPC server is created and xDS credentials are configured.
// - xDS is enabled on the client by the use of the xds:/// scheme, and xDS
//   credentials are configured.
// - Control plane is configured to send security configuration to both the
//   client and the server, pointing to the file watcher certificate provider.
//   We verify both TLS and mTLS scenarios.
func (s) TestServerSideXDS_FileWatcherCerts(t *testing.T) {
	tests := []struct {
		name     string
		secLevel e2e.SecurityLevel
	}{
		{
			name:     "tls",
			secLevel: e2e.SecurityLevelTLS,
		},
		{
			name:     "mtls",
			secLevel: e2e.SecurityLevelMTLS,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			managementServer, nodeID, bootstrapContents, resolver, cleanup1 := setupManagementServer(t)
			defer cleanup1()

			lis, cleanup2 := setupGRPCServer(t, bootstrapContents)
			defer cleanup2()

			// Grab the host and port of the server and create client side xDS
			// resources corresponding to it.
			host, port, err := hostPortFromListener(lis)
			if err != nil {
				t.Fatalf("failed to retrieve host and port of server: %v", err)
			}

			// Create xDS resources to be consumed on the client side. This
			// includes the listener, route configuration, cluster (with
			// security configuration) and endpoint resources.
			serviceName := "my-service-file-watcher-certs-" + test.name
			resources := e2e.DefaultClientResources(e2e.ResourceParams{
				DialTarget: serviceName,
				NodeID:     nodeID,
				Host:       host,
				Port:       port,
				SecLevel:   test.secLevel,
			})

			// Create an inbound xDS listener resource for the server side that
			// contains security configuration pointing to the file watcher
			// plugin.
			inboundLis := e2e.DefaultServerListener(host, port, test.secLevel)
			resources.Listeners = append(resources.Listeners, inboundLis)

			// Setup the management server with client and server resources.
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()
			if err := managementServer.Update(ctx, resources); err != nil {
				t.Fatal(err)
			}

			// Create client-side xDS credentials with an insecure fallback.
			creds, err := xdscreds.NewClientCredentials(xdscreds.ClientOptions{
				FallbackCreds: insecure.NewCredentials(),
			})
			if err != nil {
				t.Fatal(err)
			}

			// Create a ClientConn with the xds scheme and make an RPC.
			cc, err := grpc.DialContext(ctx, fmt.Sprintf("xds:///%s", serviceName), grpc.WithTransportCredentials(creds), grpc.WithResolvers(resolver))
			if err != nil {
				t.Fatalf("failed to dial local test server: %v", err)
			}
			defer cc.Close()

			client := testpb.NewTestServiceClient(cc)
			if _, err := client.EmptyCall(ctx, &testpb.Empty{}, grpc.WaitForReady(true)); err != nil {
				t.Fatalf("rpc EmptyCall() failed: %v", err)
			}
		})
	}
}

// TestServerSideXDS_SecurityConfigChange is an e2e test where xDS is enabled on
// the server-side and xdsCredentials are configured for security. The control
// plane initially does not any security configuration. This forces the
// xdsCredentials to use fallback creds, which is this case is insecure creds.
// We verify that a client connecting with TLS creds is not able to successfully
// make an RPC. The control plane then sends a listener resource with security
// configuration pointing to the use of the file_watcher plugin and we verify
// that the same client is now able to successfully make an RPC.
func (s) TestServerSideXDS_SecurityConfigChange(t *testing.T) {
	managementServer, nodeID, bootstrapContents, resolver, cleanup1 := setupManagementServer(t)
	defer cleanup1()

	lis, cleanup2 := setupGRPCServer(t, bootstrapContents)
	defer cleanup2()

	// Grab the host and port of the server and create client side xDS resources
	// corresponding to it. This contains default resources with no security
	// configuration in the Cluster resource. This should force the xDS
	// credentials on the client to use its fallback.
	host, port, err := hostPortFromListener(lis)
	if err != nil {
		t.Fatalf("failed to retrieve host and port of server: %v", err)
	}
	const serviceName = "my-service-security-config-change"
	resources := e2e.DefaultClientResources(e2e.ResourceParams{
		DialTarget: serviceName,
		NodeID:     nodeID,
		Host:       host,
		Port:       port,
		SecLevel:   e2e.SecurityLevelNone,
	})

	// Create an inbound xDS listener resource for the server side that does not
	// contain any security configuration. This should force the xDS credentials
	// on server to use its fallback.
	inboundLis := e2e.DefaultServerListener(host, port, e2e.SecurityLevelNone)
	resources.Listeners = append(resources.Listeners, inboundLis)

	// Setup the management server with client and server-side resources.
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	if err := managementServer.Update(ctx, resources); err != nil {
		t.Fatal(err)
	}

	// Create client-side xDS credentials with an insecure fallback.
	xdsCreds, err := xdscreds.NewClientCredentials(xdscreds.ClientOptions{
		FallbackCreds: insecure.NewCredentials(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create a ClientConn with the xds scheme and make a successful RPC.
	xdsCC, err := grpc.DialContext(ctx, fmt.Sprintf("xds:///%s", serviceName), grpc.WithTransportCredentials(xdsCreds), grpc.WithResolvers(resolver))
	if err != nil {
		t.Fatalf("failed to dial local test server: %v", err)
	}
	defer xdsCC.Close()

	client := testpb.NewTestServiceClient(xdsCC)
	if _, err := client.EmptyCall(ctx, &testpb.Empty{}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("rpc EmptyCall() failed: %v", err)
	}

	// Create a ClientConn with TLS creds. This should fail since the server is
	// using fallback credentials which in this case in insecure creds.
	tlsCreds := createClientTLSCredentials(t)
	tlsCC, err := grpc.DialContext(ctx, lis.Addr().String(), grpc.WithTransportCredentials(tlsCreds))
	if err != nil {
		t.Fatalf("failed to dial local test server: %v", err)
	}
	defer tlsCC.Close()

	// We don't set 'waitForReady` here since we want this call to failfast.
	client = testpb.NewTestServiceClient(tlsCC)
	if _, err := client.EmptyCall(ctx, &testpb.Empty{}); status.Code(err) != codes.Unavailable {
		t.Fatal("rpc EmptyCall() succeeded when expected to fail")
	}

	// Switch server and client side resources with ones that contain required
	// security configuration for mTLS with a file watcher certificate provider.
	resources = e2e.DefaultClientResources(e2e.ResourceParams{
		DialTarget: serviceName,
		NodeID:     nodeID,
		Host:       host,
		Port:       port,
		SecLevel:   e2e.SecurityLevelMTLS,
	})
	inboundLis = e2e.DefaultServerListener(host, port, e2e.SecurityLevelMTLS)
	resources.Listeners = append(resources.Listeners, inboundLis)
	if err := managementServer.Update(ctx, resources); err != nil {
		t.Fatal(err)
	}

	// Make another RPC with `waitForReady` set and expect this to succeed.
	if _, err := client.EmptyCall(ctx, &testpb.Empty{}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("rpc EmptyCall() failed: %v", err)
	}
}

// TestServerSideXDS_RouteConfiguration is an e2e test which verifies routing
// functionality. The xDS enabled server will be set up with route configuration
// where the route configuration has routes with the correct routing actions
// (NonForwardingAction), and the RPC's matching those routes should proceed as
// normal.
func (s) TestServerSideXDS_RouteConfiguration(t *testing.T) {
	managementServer, nodeID, bootstrapContents, resolver, cleanup1 := setupManagementServer(t)
	defer cleanup1()

	lis, cleanup2 := setupGRPCServer(t, bootstrapContents)
	defer cleanup2()

	host, port, err := hostPortFromListener(lis)
	if err != nil {
		t.Fatalf("failed to retrieve host and port of server: %v", err)
	}
	const serviceName = "my-service-fallback"
	resources := e2e.DefaultClientResources(e2e.ResourceParams{
		DialTarget: serviceName,
		NodeID:     nodeID,
		Host:       host,
		Port:       port,
		SecLevel:   e2e.SecurityLevelNone,
	})

	// Create an inbound xDS listener resource with route configuration which
	// selectively will allow RPC's through or not. This will test routing in
	// xds(Unary|Stream)Interceptors.
	vhs := []*v3routepb.VirtualHost{
		// Virtual host that will never be matched to test Virtual Host selection.
		{
			Domains: []string{"this will not match*"},
			Routes: []*v3routepb.Route{
				{
					Match: &v3routepb.RouteMatch{
						PathSpecifier: &v3routepb.RouteMatch_Prefix{Prefix: "/"},
					},
					Action: &v3routepb.Route_NonForwardingAction{},
				},
			},
		},
		// This Virtual Host will actually get matched to.
		{
			Domains: []string{"*"},
			Routes: []*v3routepb.Route{
				// A routing rule that can be selectively triggered based on properties about incoming RPC.
				{
					Match: &v3routepb.RouteMatch{
						PathSpecifier: &v3routepb.RouteMatch_Prefix{Prefix: "/grpc.testing.TestService/EmptyCall"},
						// "Fully-qualified RPC method name with leading slash. Same as :path header".
					},
					// Correct Action, so RPC's that match this route should proceed to interceptor processing.
					Action: &v3routepb.Route_NonForwardingAction{},
				},
				// This routing rule is matched the same way as the one above,
				// except has an incorrect action for the server side. However,
				// since routing chooses the first route which matches an
				// incoming RPC, this should never get invoked (iteration
				// through this route slice is deterministic).
				{
					Match: &v3routepb.RouteMatch{
						PathSpecifier: &v3routepb.RouteMatch_Prefix{Prefix: "/grpc.testing.TestService/EmptyCall"},
						// "Fully-qualified RPC method name with leading slash. Same as :path header".
					},
					// Incorrect Action, so RPC's that match this route should get denied.
					Action: &v3routepb.Route_Route{},
				},
				// Another routing rule that can be selectively triggered based on incoming RPC.
				{
					Match: &v3routepb.RouteMatch{
						PathSpecifier: &v3routepb.RouteMatch_Prefix{Prefix: "/grpc.testing.TestService/UnaryCall"},
					},
					// Wrong action (!Non_Forwarding_Action) so RPC's that match this route should get denied.
					Action: &v3routepb.Route_Route{},
				},
				// Another routing rule that can be selectively triggered based on incoming RPC.
				{
					Match: &v3routepb.RouteMatch{
						PathSpecifier: &v3routepb.RouteMatch_Prefix{Prefix: "/grpc.testing.TestService/StreamingInputCall"},
					},
					// Wrong action (!Non_Forwarding_Action) so RPC's that match this route should get denied.
					Action: &v3routepb.Route_Route{},
				},
				// Not matching route, this is be able to get invoked logically (i.e. doesn't have to match the Route configurations above).
			}},
	}
	inboundLis := &v3listenerpb.Listener{
		Name: fmt.Sprintf(e2e.ServerListenerResourceNameTemplate, net.JoinHostPort(host, strconv.Itoa(int(port)))),
		Address: &v3corepb.Address{
			Address: &v3corepb.Address_SocketAddress{
				SocketAddress: &v3corepb.SocketAddress{
					Address: host,
					PortSpecifier: &v3corepb.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		FilterChains: []*v3listenerpb.FilterChain{
			{
				Name: "v4-wildcard",
				FilterChainMatch: &v3listenerpb.FilterChainMatch{
					PrefixRanges: []*v3corepb.CidrRange{
						{
							AddressPrefix: "0.0.0.0",
							PrefixLen: &wrapperspb.UInt32Value{
								Value: uint32(0),
							},
						},
					},
					SourceType: v3listenerpb.FilterChainMatch_SAME_IP_OR_LOOPBACK,
					SourcePrefixRanges: []*v3corepb.CidrRange{
						{
							AddressPrefix: "0.0.0.0",
							PrefixLen: &wrapperspb.UInt32Value{
								Value: uint32(0),
							},
						},
					},
				},
				Filters: []*v3listenerpb.Filter{
					{
						Name: "filter-1",
						ConfigType: &v3listenerpb.Filter_TypedConfig{
							TypedConfig: testutils.MarshalAny(&v3httppb.HttpConnectionManager{
								HttpFilters: []*v3httppb.HttpFilter{e2e.HTTPFilter("router", &v3routerpb.Router{})},
								RouteSpecifier: &v3httppb.HttpConnectionManager_RouteConfig{
									RouteConfig: &v3routepb.RouteConfiguration{
										Name:         "routeName",
										VirtualHosts: vhs,
									},
								},
							}),
						},
					},
				},
			},
			{
				Name: "v6-wildcard",
				FilterChainMatch: &v3listenerpb.FilterChainMatch{
					PrefixRanges: []*v3corepb.CidrRange{
						{
							AddressPrefix: "::",
							PrefixLen: &wrapperspb.UInt32Value{
								Value: uint32(0),
							},
						},
					},
					SourceType: v3listenerpb.FilterChainMatch_SAME_IP_OR_LOOPBACK,
					SourcePrefixRanges: []*v3corepb.CidrRange{
						{
							AddressPrefix: "::",
							PrefixLen: &wrapperspb.UInt32Value{
								Value: uint32(0),
							},
						},
					},
				},
				Filters: []*v3listenerpb.Filter{
					{
						Name: "filter-1",
						ConfigType: &v3listenerpb.Filter_TypedConfig{
							TypedConfig: testutils.MarshalAny(&v3httppb.HttpConnectionManager{
								HttpFilters: []*v3httppb.HttpFilter{e2e.HTTPFilter("router", &v3routerpb.Router{})},
								RouteSpecifier: &v3httppb.HttpConnectionManager_RouteConfig{
									RouteConfig: &v3routepb.RouteConfiguration{
										Name:         "routeName",
										VirtualHosts: vhs,
									},
								},
							}),
						},
					},
				},
			},
		},
	}
	resources.Listeners = append(resources.Listeners, inboundLis)

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	// Setup the management server with client and server-side resources.
	if err := managementServer.Update(ctx, resources); err != nil {
		t.Fatal(err)
	}

	cc, err := grpc.DialContext(ctx, fmt.Sprintf("xds:///%s", serviceName), grpc.WithInsecure(), grpc.WithResolvers(resolver))
	if err != nil {
		t.Fatalf("failed to dial local test server: %v", err)
	}
	defer cc.Close()

	client := testpb.NewTestServiceClient(cc)

	// This Empty Call should match to a route with a correct action
	// (NonForwardingAction). Thus, this RPC should proceed as normal. There is
	// a routing rule that this RPC would match to that has an incorrect action,
	// but the server should only use the first route matched to with the
	// correct action.
	if _, err = client.EmptyCall(ctx, &testpb.Empty{}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("rpc EmptyCall() failed: %v", err)
	}

	// This Unary Call should match to a route with an incorrect action. Thus,
	// this RPC should not go through as per A36, and this call should receive
	// an error with codes.Unavailable.
	if _, err = client.UnaryCall(ctx, &testpb.SimpleRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("client.UnaryCall() = _, %v, want _, error code %s", err, codes.Unavailable)
	}

	// This Streaming Call should match to a route with an incorrect action.
	// Thus, this RPC should not go through as per A36, and this call should
	// receive an error with codes.Unavailable.
	stream, err := client.StreamingInputCall(ctx)
	if err != nil {
		t.Fatalf("StreamingInputCall(_) = _, %v, want <nil>", err)
	}
	if _, err = stream.CloseAndRecv(); status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "the incoming RPC matched to a route that was not of action type non forwarding") {
		t.Fatalf("streaming RPC should have been denied")
	}

	// This Full Duplex should not match to a route, and thus should return an
	// error and not proceed.
	dStream, err := client.FullDuplexCall(ctx)
	if err != nil {
		t.Fatalf("FullDuplexCall(_) = _, %v, want <nil>", err)
	}
	if _, err = dStream.Recv(); status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "the incoming RPC did not match a configured Route") {
		t.Fatalf("streaming RPC should have been denied")
	}
}
