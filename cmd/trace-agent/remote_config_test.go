// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

func TestConfigEndpoint(t *testing.T) {
	defer func(old string) { features.Set(old) }(strings.Join(features.All(), ","))

	var tcs = []struct {
		name               string
		reqBody            string
		expectedStatusCode int
		enabled            bool
		valid              bool
		response           string
	}{
		{
			name:               "bad",
			enabled:            true,
			expectedStatusCode: http.StatusBadRequest,
			response:           "unexpected end of JSON input\n",
		},
		{
			name:    "valid",
			reqBody: `{"client":{"id":"test_client"}}`,

			enabled:            true,
			valid:              true,
			expectedStatusCode: http.StatusOK,
			response:           `{"targets":{"version":1,"raw":"dGVzdA=="}}`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			grpc := mockAgentSecureServer{}
			rcv := api.NewHTTPReceiver(config.New(), sampler.NewDynamicConfig(), make(chan *api.Payload, 5000), nil)
			mux := http.NewServeMux()
			mux.Handle("/v0.7/config", remoteConfigHandler(rcv, &grpc, ""))
			server := httptest.NewServer(mux)
			if tc.valid {
				var request pbgo.ClientGetConfigsRequest
				err := json.Unmarshal([]byte(tc.reqBody), &request)
				assert.NoError(err)
				grpc.On("ClientGetConfigs", mock.Anything, &request, mock.Anything).Return(&pbgo.ClientGetConfigsResponse{Targets: &pbgo.TopMeta{Version: 1, Raw: []byte("test")}}, nil)
			}
			req, _ := http.NewRequest("POST", server.URL+"/v0.7/config", strings.NewReader(tc.reqBody))
			req.Header.Set("Content-Type", "application/msgpack")
			resp, err := http.DefaultClient.Do(req)
			assert.Nil(err)
			body, err := ioutil.ReadAll(resp.Body)
			assert.Nil(err)
			assert.Equal(tc.expectedStatusCode, resp.StatusCode)
			assert.Equal(tc.response, string(body))
		})
	}
}

type mockAgentSecureServer struct {
	pbgo.AgentSecureClient
	mock.Mock
}

func (a *mockAgentSecureServer) TaggerStreamEntities(ctx context.Context, in *pbgo.StreamTagsRequest, opts ...grpc.CallOption) (pbgo.AgentSecure_TaggerStreamEntitiesClient, error) {
	args := a.Called(ctx, in, opts)
	return args.Get(0).(pbgo.AgentSecure_TaggerStreamEntitiesClient), args.Error(1)
}

func (a *mockAgentSecureServer) TaggerFetchEntity(ctx context.Context, in *pbgo.FetchEntityRequest, opts ...grpc.CallOption) (*pbgo.FetchEntityResponse, error) {
	args := a.Called(ctx, in, opts)
	return args.Get(0).(*pbgo.FetchEntityResponse), args.Error(1)
}

func (a *mockAgentSecureServer) DogstatsdCaptureTrigger(ctx context.Context, in *pbgo.CaptureTriggerRequest, opts ...grpc.CallOption) (*pbgo.CaptureTriggerResponse, error) {
	args := a.Called(ctx, in, opts)
	return args.Get(0).(*pbgo.CaptureTriggerResponse), args.Error(1)
}

func (a *mockAgentSecureServer) DogstatsdSetTaggerState(ctx context.Context, in *pbgo.TaggerState, opts ...grpc.CallOption) (*pbgo.TaggerStateResponse, error) {
	args := a.Called(ctx, in, opts)
	return args.Get(0).(*pbgo.TaggerStateResponse), args.Error(1)
}

func (a *mockAgentSecureServer) ClientGetConfigs(ctx context.Context, in *pbgo.ClientGetConfigsRequest, opts ...grpc.CallOption) (*pbgo.ClientGetConfigsResponse, error) {
	args := a.Called(ctx, in, opts)
	return args.Get(0).(*pbgo.ClientGetConfigsResponse), args.Error(1)
}
