// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: LicenseRef-ONF-Member-1.0

package subscriptiontask

import (
	epapi "github.com/onosproject/onos-e2sub/api/e2/endpoint/v1beta1"
	subapi "github.com/onosproject/onos-e2sub/api/e2/subscription/v1beta1"
	"github.com/onosproject/onos-ric-sdk-go/pkg/e2"
	"io"

	subtaskapi "github.com/onosproject/onos-e2sub/api/e2/task/v1beta1"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var log = logging.GetLogger("e2", "subscription", "client")

// ListOption is an option for filtering List calls
type ListOption interface {
	applyList(*listOptions)
}

type listOptions struct {
	subscriptionID subapi.ID
	endpointID     epapi.ID
}

// WatchOption is an option for filtering Watch calls
type WatchOption interface {
	applyWatch(*watchOptions)
}

type watchOptions struct {
	subscriptionID subapi.ID
	endpointID     epapi.ID
}

// FilterOption is an option for filtering List/Watch calls
type FilterOption interface {
	ListOption
	WatchOption
}

// WithSubscriptionID creates an option for filtering by subscription ID
func WithSubscriptionID(id subapi.ID) FilterOption {
	return &filterSubscriptionOption{
		subID: id,
	}
}

type filterSubscriptionOption struct {
	subID subapi.ID
}

func (o *filterSubscriptionOption) applyList(options *listOptions) {
	options.subscriptionID = o.subID
}

func (o *filterSubscriptionOption) applyWatch(options *watchOptions) {
	options.subscriptionID = o.subID
}

// WithEndpointID creates an option for filtering by endpoint ID
func WithEndpointID(id epapi.ID) FilterOption {
	return &filterEndpointOption{
		epID: id,
	}
}

type filterEndpointOption struct {
	epID epapi.ID
}

func (o *filterEndpointOption) applyList(options *listOptions) {
	options.endpointID = o.epID
}

func (o *filterEndpointOption) applyWatch(options *watchOptions) {
	options.endpointID = o.epID
}

// Client is an E2 subscription service client interface
type Client interface {
	// Get returns a subscription based on a given subscription ID
	Get(ctx context.Context, id subtaskapi.ID) (*subtaskapi.SubscriptionTask, error)

	// List returns the list of existing subscriptions
	List(ctx context.Context, opts ...ListOption) ([]subtaskapi.SubscriptionTask, error)

	// Watch watches the subscription changes
	Watch(ctx context.Context, ch chan<- subtaskapi.Event, opts ...WatchOption) error
}

// localClient subscription client
type localClient struct {
	conn   *grpc.ClientConn
	client subtaskapi.E2SubscriptionTaskServiceClient
}

// Destination determines subscription service endpoint
type Destination struct {
	// Addrs a slice of addresses by which a subscription service may be reached.
	Addrs []string
}

// NewClient creates a new subscribe service client
func NewClient(ctx context.Context, dst Destination) (Client, error) {
	tlsConfig, err := e2.GetClientCredentials()
	if err != nil {
		return &localClient{}, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	}

	conn, err := grpc.DialContext(ctx, dst.Addrs[0], opts...)
	if err != nil {
		return &localClient{}, err
	}

	cl := subtaskapi.NewE2SubscriptionTaskServiceClient(conn)

	client := localClient{
		client: cl,
		conn:   conn,
	}

	return &client, nil
}

// Get returns information about a subscription
func (c *localClient) Get(ctx context.Context, id subtaskapi.ID) (*subtaskapi.SubscriptionTask, error) {
	req := &subtaskapi.GetSubscriptionTaskRequest{
		ID: id,
	}

	resp, err := c.client.GetSubscriptionTask(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Task, nil
}

// List returns the list of all subscriptions
func (c *localClient) List(ctx context.Context, opts ...ListOption) ([]subtaskapi.SubscriptionTask, error) {
	options := &listOptions{}
	for _, opt := range opts {
		opt.applyList(options)
	}

	req := &subtaskapi.ListSubscriptionTasksRequest{
		SubscriptionID: options.subscriptionID,
		EndpointID:     options.endpointID,
	}

	resp, err := c.client.ListSubscriptionTasks(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Tasks, nil
}

// Watch watches for changes in the set of subscriptions
func (c *localClient) Watch(ctx context.Context, ch chan<- subtaskapi.Event, opts ...WatchOption) error {
	options := &watchOptions{}
	for _, opt := range opts {
		opt.applyWatch(options)
	}

	req := subtaskapi.WatchSubscriptionTasksRequest{
		SubscriptionID: options.subscriptionID,
		EndpointID:     options.endpointID,
	}

	stream, err := c.client.WatchSubscriptionTasks(ctx, &req)
	if err != nil {
		close(ch)
		return err
	}

	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF || err == context.Canceled {
				close(ch)
				break
			}

			if err != nil {
				log.Error("an error occurred in receiving subscription changes", err)
			}

			ch <- resp.Event

		}

	}()
	return nil
}

// Close closes the client connection
func (c *localClient) Close() error {
	return c.conn.Close()
}

var _ Client = &localClient{}