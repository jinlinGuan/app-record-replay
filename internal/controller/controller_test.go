//
// Copyright (c) 2023 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appMocks "github.com/edgexfoundry/app-functions-sdk-go/v3/pkg/interfaces/mocks"
	"github.com/edgexfoundry/app-record-replay/internal/interfaces/mocks"
	"github.com/edgexfoundry/app-record-replay/pkg/dtos"
	"github.com/edgexfoundry/go-mod-core-contracts/v3/clients/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	target, _, _ := createTargetAndMocks()

	require.NotNil(t, target)
	require.NotNil(t, target)
	assert.NotNil(t, target.appSdk)
	assert.NotNil(t, target.lc)
	assert.NotNil(t, target.dataManager)
}

func TestHttpController_AddRoutes_Success(t *testing.T) {
	target, _, mockSdk := createTargetAndMocks()
	mockSdk.On("AddRoute", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := target.AddRoutes()
	require.NoError(t, err)
}

func TestHttpController_AddRoutes_Error(t *testing.T) {
	tests := []struct {
		Name   string
		Route  string
		Method string
	}{
		{"Start Recording", recordRoute, http.MethodPost},
		{"Cancel Recording", recordRoute, http.MethodDelete},
		{"Recording Status", recordRoute, http.MethodGet},

		{"Start Replay", replayRoute, http.MethodPost},
		{"Cancel Replay", replayRoute, http.MethodDelete},
		{"Replay Status", replayRoute, http.MethodGet},

		{"Export", dataRoute, http.MethodGet},
		{"Import", dataRoute, http.MethodPost},
	}

	expectedError := errors.New("AddRoute error")
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockSdk := &appMocks.ApplicationService{}
			mockSdk.On("AddRoute", test.Route, mock.Anything, test.Method).Return(expectedError)
			mockSdk.On("AddRoute", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockSdk.On("LoggingClient").Return(logger.NewMockClient())

			target := New(nil, mockSdk)

			err := target.AddRoutes()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.Route)
			assert.Contains(t, err.Error(), test.Method)
			assert.Contains(t, err.Error(), expectedError.Error())
		})
	}
}

func TestHttpController_StartRecording(t *testing.T) {
	target, mockDataManager, _ := createTargetAndMocks()

	handler := http.HandlerFunc(target.startRecording)

	emptyRequestDTO := dtos.RecordRequest{}

	validRequestDTO := dtos.RecordRequest{
		Duration:              1 * time.Minute,
		EventLimit:            10,
		IncludeDeviceProfiles: nil,
		IncludeDevices:        nil,
		IncludeSources:        nil,
		ExcludeDeviceProfiles: nil,
		ExcludeDevices:        nil,
		ExcludeSources:        nil,
	}
	badDurationRequestDTO := dtos.RecordRequest{
		Duration:              -99,
		EventLimit:            10,
		IncludeDeviceProfiles: nil,
		IncludeDevices:        nil,
		IncludeSources:        nil,
		ExcludeDeviceProfiles: nil,
		ExcludeDevices:        nil,
		ExcludeSources:        nil,
	}

	badEventLimitRequestDTO := dtos.RecordRequest{
		Duration:              0,
		EventLimit:            -99,
		IncludeDeviceProfiles: nil,
		IncludeDevices:        nil,
		IncludeSources:        nil,
		ExcludeDeviceProfiles: nil,
		ExcludeDevices:        nil,
		ExcludeSources:        nil,
	}

	tests := []struct {
		Name                         string
		Input                        []byte
		MockDataManagerStartResponse error
		ExpectedStatus               int
		ExpectedMessage              string
	}{
		{"Success", marshal(t, validRequestDTO), nil, http.StatusAccepted, ""},
		{"Recording failed", marshal(t, validRequestDTO), errors.New("recording failed"), http.StatusInternalServerError, failedRecording},
		{"No Input", nil, nil, http.StatusBadRequest, failedRequestJSON},
		{"Bad JSON Input", []byte("bad input"), nil, http.StatusBadRequest, failedRequestJSON},
		{"Empty DTO Input", marshal(t, emptyRequestDTO), nil, http.StatusBadRequest, failedRecordRequestValidate},
		{"Bad Duration", marshal(t, badDurationRequestDTO), nil, http.StatusBadRequest, failedRecordDurationValidate},
		{"Bad Event Limit", marshal(t, badEventLimitRequestDTO), nil, http.StatusBadRequest, failedRecordEventLimitValidate},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockDataManager.On("StartRecording", &validRequestDTO).Return(test.MockDataManagerStartResponse).Once()
			req, err := http.NewRequest(http.MethodPost, recordRoute, bytes.NewReader(test.Input))
			require.NoError(t, err)

			testRecorder := httptest.NewRecorder()
			handler.ServeHTTP(testRecorder, req)

			require.Equal(t, test.ExpectedStatus, testRecorder.Code)
			assert.Contains(t, testRecorder.Body.String(), test.ExpectedMessage)
		})
	}
}

func TestHttpController_RecordingStatus(t *testing.T) {
	target, mockDataManager, _ := createTargetAndMocks()

	handler := http.HandlerFunc(target.recordingStatus)

	inProgressRecordStatus := dtos.RecordStatus{
		InProgress: true,
		EventCount: 10,
		Duration:   time.Second,
	}
	tests := []struct {
		Name                         string
		MockDataManagerStartResponse error
		ExpectedResponse             *dtos.RecordStatus
		ExpectedStatus               int
		ExpectedError                error
	}{
		{
			Name:             "Valid in progress status test",
			ExpectedResponse: &inProgressRecordStatus,
			ExpectedStatus:   http.StatusOK,
			ExpectedError:    nil,
		},
		{
			Name:           "retrieve status error",
			ExpectedError:  errors.New("failed"),
			ExpectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockDataManager.On("RecordingStatus").Return(test.ExpectedResponse, test.ExpectedError).Once()
			req, err := http.NewRequest(http.MethodGet, recordRoute, nil)
			require.NoError(t, err)

			testRecorder := httptest.NewRecorder()
			handler.ServeHTTP(testRecorder, req)

			require.Equal(t, test.ExpectedStatus, testRecorder.Code)
			if test.ExpectedStatus != http.StatusOK {
				return
			}

			require.NotNil(t, testRecorder.Body)
			actualResponse := dtos.RecordStatus{}
			err = json.Unmarshal(testRecorder.Body.Bytes(), &actualResponse)
			require.NoError(t, err)

			require.Equal(t, test.ExpectedResponse, &actualResponse)
		})
	}
}

func TestHttpController_CancelRecording(t *testing.T) {
	target, mockDataManager, _ := createTargetAndMocks()

	handler := http.HandlerFunc(target.cancelRecording)

	tests := []struct {
		Name           string
		ExpectedStatus int
		ExpectedError  error
	}{
		{"Valid", http.StatusAccepted, nil},
		{"Error", http.StatusInternalServerError, errors.New("failed")},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockDataManager.On("CancelRecording").Return(test.ExpectedError).Once()

			req, err := http.NewRequest(http.MethodGet, recordRoute, nil)
			require.NoError(t, err)

			testRecorder := httptest.NewRecorder()
			handler.ServeHTTP(testRecorder, req)

			require.Equal(t, test.ExpectedStatus, testRecorder.Code)
		})
	}
}

func TestHttpController_StartReplay(t *testing.T) {
	// TODO: Implement using TDD
}

func TestHttpController_ReplayStatus(t *testing.T) {
	// TODO: Implement using TDD
}

func TestHttpController_CancelReplay(t *testing.T) {
	target, mockDataManager, _ := createTargetAndMocks()

	handler := http.HandlerFunc(target.cancelReplay)

	tests := []struct {
		Name           string
		ExpectedStatus int
		ExpectedError  error
	}{
		{"Valid", http.StatusAccepted, nil},
		{"Error", http.StatusInternalServerError, errors.New("failed")},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			mockDataManager.On("CancelReplay").Return(test.ExpectedError).Once()

			req, err := http.NewRequest(http.MethodGet, recordRoute, nil)
			require.NoError(t, err)

			testRecorder := httptest.NewRecorder()
			handler.ServeHTTP(testRecorder, req)

			require.Equal(t, test.ExpectedStatus, testRecorder.Code)
		})
	}
}

func TestHttpController_ExportRecordedData(t *testing.T) {
	// TODO: Implement using TDD
}

func TestHttpController_ImportRecordedData(t *testing.T) {
	// TODO: Implement using TDD
}

func marshal(t *testing.T, v any) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func createTargetAndMocks() (*httpController, *mocks.DataManager, *appMocks.ApplicationService) {
	mockDataManager := &mocks.DataManager{}
	mockSdk := &appMocks.ApplicationService{}
	mockSdk.On("LoggingClient").Return(logger.NewMockClient())

	target := New(mockDataManager, mockSdk).(*httpController)
	return target, mockDataManager, mockSdk
}
