// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client (interfaces: Service)

// Package mock_client is a generated GoMock package.
package mock_client

import (
	context "context"
	http "net/http"
	reflect "reflect"

	client "github.com/cockroachdb/cockroach-cloud-sdk-go/pkg/client"
	gomock "github.com/golang/mock/gomock"
)

// MockService is a mock of Service interface.
type MockService struct {
	ctrl     *gomock.Controller
	recorder *MockServiceMockRecorder
}

// MockServiceMockRecorder is the mock recorder for MockService.
type MockServiceMockRecorder struct {
	mock *MockService
}

// NewMockService creates a new mock instance.
func NewMockService(ctrl *gomock.Controller) *MockService {
	mock := &MockService{ctrl: ctrl}
	mock.recorder = &MockServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockService) EXPECT() *MockServiceMockRecorder {
	return m.recorder
}

// AddAllowlistEntry mocks base method.
func (m *MockService) AddAllowlistEntry(arg0 context.Context, arg1 string, arg2 *client.AllowlistEntry) (*client.AllowlistEntry, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddAllowlistEntry", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.AllowlistEntry)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// AddAllowlistEntry indicates an expected call of AddAllowlistEntry.
func (mr *MockServiceMockRecorder) AddAllowlistEntry(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddAllowlistEntry", reflect.TypeOf((*MockService)(nil).AddAllowlistEntry), arg0, arg1, arg2)
}

// AddAllowlistEntry2 mocks base method.
func (m *MockService) AddAllowlistEntry2(arg0 context.Context, arg1, arg2 string, arg3 int32, arg4 *client.AllowlistEntry1) (*client.AllowlistEntry, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddAllowlistEntry2", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].(*client.AllowlistEntry)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// AddAllowlistEntry2 indicates an expected call of AddAllowlistEntry2.
func (mr *MockServiceMockRecorder) AddAllowlistEntry2(arg0, arg1, arg2, arg3, arg4 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddAllowlistEntry2", reflect.TypeOf((*MockService)(nil).AddAllowlistEntry2), arg0, arg1, arg2, arg3, arg4)
}

// CreateCluster mocks base method.
func (m *MockService) CreateCluster(arg0 context.Context, arg1 *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateCluster", arg0, arg1)
	ret0, _ := ret[0].(*client.Cluster)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// CreateCluster indicates an expected call of CreateCluster.
func (mr *MockServiceMockRecorder) CreateCluster(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateCluster", reflect.TypeOf((*MockService)(nil).CreateCluster), arg0, arg1)
}

// CreateDatabase mocks base method.
func (m *MockService) CreateDatabase(arg0 context.Context, arg1 string, arg2 *client.CreateDatabaseRequest) (*client.ApiDatabase, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateDatabase", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ApiDatabase)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// CreateDatabase indicates an expected call of CreateDatabase.
func (mr *MockServiceMockRecorder) CreateDatabase(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateDatabase", reflect.TypeOf((*MockService)(nil).CreateDatabase), arg0, arg1, arg2)
}

// CreatePrivateEndpointServices mocks base method.
func (m *MockService) CreatePrivateEndpointServices(arg0 context.Context, arg1 string, arg2 *map[string]interface{}) (*client.PrivateEndpointServices, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreatePrivateEndpointServices", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.PrivateEndpointServices)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// CreatePrivateEndpointServices indicates an expected call of CreatePrivateEndpointServices.
func (mr *MockServiceMockRecorder) CreatePrivateEndpointServices(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreatePrivateEndpointServices", reflect.TypeOf((*MockService)(nil).CreatePrivateEndpointServices), arg0, arg1, arg2)
}

// CreateSQLUser mocks base method.
func (m *MockService) CreateSQLUser(arg0 context.Context, arg1 string, arg2 *client.CreateSQLUserRequest) (*client.SQLUser, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateSQLUser", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.SQLUser)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// CreateSQLUser indicates an expected call of CreateSQLUser.
func (mr *MockServiceMockRecorder) CreateSQLUser(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateSQLUser", reflect.TypeOf((*MockService)(nil).CreateSQLUser), arg0, arg1, arg2)
}

// DeleteAllowlistEntry mocks base method.
func (m *MockService) DeleteAllowlistEntry(arg0 context.Context, arg1, arg2 string, arg3 int32) (*client.AllowlistEntry, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteAllowlistEntry", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(*client.AllowlistEntry)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// DeleteAllowlistEntry indicates an expected call of DeleteAllowlistEntry.
func (mr *MockServiceMockRecorder) DeleteAllowlistEntry(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteAllowlistEntry", reflect.TypeOf((*MockService)(nil).DeleteAllowlistEntry), arg0, arg1, arg2, arg3)
}

// DeleteCluster mocks base method.
func (m *MockService) DeleteCluster(arg0 context.Context, arg1 string) (*client.Cluster, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteCluster", arg0, arg1)
	ret0, _ := ret[0].(*client.Cluster)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// DeleteCluster indicates an expected call of DeleteCluster.
func (mr *MockServiceMockRecorder) DeleteCluster(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteCluster", reflect.TypeOf((*MockService)(nil).DeleteCluster), arg0, arg1)
}

// DeleteDatabase mocks base method.
func (m *MockService) DeleteDatabase(arg0 context.Context, arg1, arg2 string) (*client.ApiDatabase, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteDatabase", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ApiDatabase)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// DeleteDatabase indicates an expected call of DeleteDatabase.
func (mr *MockServiceMockRecorder) DeleteDatabase(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteDatabase", reflect.TypeOf((*MockService)(nil).DeleteDatabase), arg0, arg1, arg2)
}

// DeleteLogExport mocks base method.
func (m *MockService) DeleteLogExport(arg0 context.Context, arg1 string) (*client.LogExportClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteLogExport", arg0, arg1)
	ret0, _ := ret[0].(*client.LogExportClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// DeleteLogExport indicates an expected call of DeleteLogExport.
func (mr *MockServiceMockRecorder) DeleteLogExport(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteLogExport", reflect.TypeOf((*MockService)(nil).DeleteLogExport), arg0, arg1)
}

// DeleteSQLUser mocks base method.
func (m *MockService) DeleteSQLUser(arg0 context.Context, arg1, arg2 string) (*client.SQLUser, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteSQLUser", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.SQLUser)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// DeleteSQLUser indicates an expected call of DeleteSQLUser.
func (mr *MockServiceMockRecorder) DeleteSQLUser(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteSQLUser", reflect.TypeOf((*MockService)(nil).DeleteSQLUser), arg0, arg1, arg2)
}

// EditDatabase mocks base method.
func (m *MockService) EditDatabase(arg0 context.Context, arg1 string, arg2 *client.UpdateDatabaseRequest) (*client.ApiDatabase, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EditDatabase", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ApiDatabase)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// EditDatabase indicates an expected call of EditDatabase.
func (mr *MockServiceMockRecorder) EditDatabase(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EditDatabase", reflect.TypeOf((*MockService)(nil).EditDatabase), arg0, arg1, arg2)
}

// EnableCMEKSpec mocks base method.
func (m *MockService) EnableCMEKSpec(arg0 context.Context, arg1 string, arg2 *client.CMEKClusterSpecification) (*client.CMEKClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnableCMEKSpec", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.CMEKClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// EnableCMEKSpec indicates an expected call of EnableCMEKSpec.
func (mr *MockServiceMockRecorder) EnableCMEKSpec(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnableCMEKSpec", reflect.TypeOf((*MockService)(nil).EnableCMEKSpec), arg0, arg1, arg2)
}

// EnableLogExport mocks base method.
func (m *MockService) EnableLogExport(arg0 context.Context, arg1 string, arg2 *client.EnableLogExportRequest) (*client.LogExportClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnableLogExport", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.LogExportClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// EnableLogExport indicates an expected call of EnableLogExport.
func (mr *MockServiceMockRecorder) EnableLogExport(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnableLogExport", reflect.TypeOf((*MockService)(nil).EnableLogExport), arg0, arg1, arg2)
}

// GetCMEKClusterInfo mocks base method.
func (m *MockService) GetCMEKClusterInfo(arg0 context.Context, arg1 string) (*client.CMEKClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCMEKClusterInfo", arg0, arg1)
	ret0, _ := ret[0].(*client.CMEKClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetCMEKClusterInfo indicates an expected call of GetCMEKClusterInfo.
func (mr *MockServiceMockRecorder) GetCMEKClusterInfo(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCMEKClusterInfo", reflect.TypeOf((*MockService)(nil).GetCMEKClusterInfo), arg0, arg1)
}

// GetCluster mocks base method.
func (m *MockService) GetCluster(arg0 context.Context, arg1 string) (*client.Cluster, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCluster", arg0, arg1)
	ret0, _ := ret[0].(*client.Cluster)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetCluster indicates an expected call of GetCluster.
func (mr *MockServiceMockRecorder) GetCluster(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCluster", reflect.TypeOf((*MockService)(nil).GetCluster), arg0, arg1)
}

// GetInvoice mocks base method.
func (m *MockService) GetInvoice(arg0 context.Context, arg1 string) (*client.Invoice, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetInvoice", arg0, arg1)
	ret0, _ := ret[0].(*client.Invoice)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetInvoice indicates an expected call of GetInvoice.
func (mr *MockServiceMockRecorder) GetInvoice(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetInvoice", reflect.TypeOf((*MockService)(nil).GetInvoice), arg0, arg1)
}

// GetLogExportInfo mocks base method.
func (m *MockService) GetLogExportInfo(arg0 context.Context, arg1 string) (*client.LogExportClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetLogExportInfo", arg0, arg1)
	ret0, _ := ret[0].(*client.LogExportClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetLogExportInfo indicates an expected call of GetLogExportInfo.
func (mr *MockServiceMockRecorder) GetLogExportInfo(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetLogExportInfo", reflect.TypeOf((*MockService)(nil).GetLogExportInfo), arg0, arg1)
}

// ListAllowlistEntries mocks base method.
func (m *MockService) ListAllowlistEntries(arg0 context.Context, arg1 string, arg2 *client.ListAllowlistEntriesOptions) (*client.ListAllowlistEntriesResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListAllowlistEntries", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ListAllowlistEntriesResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListAllowlistEntries indicates an expected call of ListAllowlistEntries.
func (mr *MockServiceMockRecorder) ListAllowlistEntries(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListAllowlistEntries", reflect.TypeOf((*MockService)(nil).ListAllowlistEntries), arg0, arg1, arg2)
}

// ListAvailableRegions mocks base method.
func (m *MockService) ListAvailableRegions(arg0 context.Context, arg1 *client.ListAvailableRegionsOptions) (*client.ListAvailableRegionsResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListAvailableRegions", arg0, arg1)
	ret0, _ := ret[0].(*client.ListAvailableRegionsResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListAvailableRegions indicates an expected call of ListAvailableRegions.
func (mr *MockServiceMockRecorder) ListAvailableRegions(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListAvailableRegions", reflect.TypeOf((*MockService)(nil).ListAvailableRegions), arg0, arg1)
}

// ListAwsEndpointConnections mocks base method.
func (m *MockService) ListAwsEndpointConnections(arg0 context.Context, arg1 string) (*client.AwsEndpointConnections, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListAwsEndpointConnections", arg0, arg1)
	ret0, _ := ret[0].(*client.AwsEndpointConnections)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListAwsEndpointConnections indicates an expected call of ListAwsEndpointConnections.
func (mr *MockServiceMockRecorder) ListAwsEndpointConnections(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListAwsEndpointConnections", reflect.TypeOf((*MockService)(nil).ListAwsEndpointConnections), arg0, arg1)
}

// ListClusterNodes mocks base method.
func (m *MockService) ListClusterNodes(arg0 context.Context, arg1 string, arg2 *client.ListClusterNodesOptions) (*client.ListClusterNodesResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListClusterNodes", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ListClusterNodesResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListClusterNodes indicates an expected call of ListClusterNodes.
func (mr *MockServiceMockRecorder) ListClusterNodes(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListClusterNodes", reflect.TypeOf((*MockService)(nil).ListClusterNodes), arg0, arg1, arg2)
}

// ListClusters mocks base method.
func (m *MockService) ListClusters(arg0 context.Context, arg1 *client.ListClustersOptions) (*client.ListClustersResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListClusters", arg0, arg1)
	ret0, _ := ret[0].(*client.ListClustersResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListClusters indicates an expected call of ListClusters.
func (mr *MockServiceMockRecorder) ListClusters(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListClusters", reflect.TypeOf((*MockService)(nil).ListClusters), arg0, arg1)
}

// ListDatabases mocks base method.
func (m *MockService) ListDatabases(arg0 context.Context, arg1 string, arg2 *client.ListDatabasesOptions) (*client.ApiListDatabasesResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListDatabases", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ApiListDatabasesResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListDatabases indicates an expected call of ListDatabases.
func (mr *MockServiceMockRecorder) ListDatabases(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListDatabases", reflect.TypeOf((*MockService)(nil).ListDatabases), arg0, arg1, arg2)
}

// ListInvoices mocks base method.
func (m *MockService) ListInvoices(arg0 context.Context) (*client.ListInvoicesResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListInvoices", arg0)
	ret0, _ := ret[0].(*client.ListInvoicesResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListInvoices indicates an expected call of ListInvoices.
func (mr *MockServiceMockRecorder) ListInvoices(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListInvoices", reflect.TypeOf((*MockService)(nil).ListInvoices), arg0)
}

// ListPrivateEndpointServices mocks base method.
func (m *MockService) ListPrivateEndpointServices(arg0 context.Context, arg1 string) (*client.PrivateEndpointServices, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListPrivateEndpointServices", arg0, arg1)
	ret0, _ := ret[0].(*client.PrivateEndpointServices)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListPrivateEndpointServices indicates an expected call of ListPrivateEndpointServices.
func (mr *MockServiceMockRecorder) ListPrivateEndpointServices(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListPrivateEndpointServices", reflect.TypeOf((*MockService)(nil).ListPrivateEndpointServices), arg0, arg1)
}

// ListSQLUsers mocks base method.
func (m *MockService) ListSQLUsers(arg0 context.Context, arg1 string, arg2 *client.ListSQLUsersOptions) (*client.ListSQLUsersResponse, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListSQLUsers", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.ListSQLUsersResponse)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ListSQLUsers indicates an expected call of ListSQLUsers.
func (mr *MockServiceMockRecorder) ListSQLUsers(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListSQLUsers", reflect.TypeOf((*MockService)(nil).ListSQLUsers), arg0, arg1, arg2)
}

// SetAwsEndpointConnectionState mocks base method.
func (m *MockService) SetAwsEndpointConnectionState(arg0 context.Context, arg1, arg2 string, arg3 *client.CockroachCloudSetAwsEndpointConnectionStateRequest) (*client.AwsEndpointConnection, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetAwsEndpointConnectionState", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(*client.AwsEndpointConnection)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// SetAwsEndpointConnectionState indicates an expected call of SetAwsEndpointConnectionState.
func (mr *MockServiceMockRecorder) SetAwsEndpointConnectionState(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetAwsEndpointConnectionState", reflect.TypeOf((*MockService)(nil).SetAwsEndpointConnectionState), arg0, arg1, arg2, arg3)
}

// UpdateAllowlistEntry mocks base method.
func (m *MockService) UpdateAllowlistEntry(arg0 context.Context, arg1, arg2 string, arg3 int32, arg4 *client.AllowlistEntry1, arg5 *client.UpdateAllowlistEntryOptions) (*client.AllowlistEntry, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateAllowlistEntry", arg0, arg1, arg2, arg3, arg4, arg5)
	ret0, _ := ret[0].(*client.AllowlistEntry)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateAllowlistEntry indicates an expected call of UpdateAllowlistEntry.
func (mr *MockServiceMockRecorder) UpdateAllowlistEntry(arg0, arg1, arg2, arg3, arg4, arg5 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateAllowlistEntry", reflect.TypeOf((*MockService)(nil).UpdateAllowlistEntry), arg0, arg1, arg2, arg3, arg4, arg5)
}

// UpdateCMEKSpec mocks base method.
func (m *MockService) UpdateCMEKSpec(arg0 context.Context, arg1 string, arg2 *client.CMEKClusterSpecification) (*client.CMEKClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateCMEKSpec", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.CMEKClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateCMEKSpec indicates an expected call of UpdateCMEKSpec.
func (mr *MockServiceMockRecorder) UpdateCMEKSpec(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateCMEKSpec", reflect.TypeOf((*MockService)(nil).UpdateCMEKSpec), arg0, arg1, arg2)
}

// UpdateCMEKStatus mocks base method.
func (m *MockService) UpdateCMEKStatus(arg0 context.Context, arg1 string, arg2 *client.UpdateCMEKStatusRequest) (*client.CMEKClusterInfo, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateCMEKStatus", arg0, arg1, arg2)
	ret0, _ := ret[0].(*client.CMEKClusterInfo)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateCMEKStatus indicates an expected call of UpdateCMEKStatus.
func (mr *MockServiceMockRecorder) UpdateCMEKStatus(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateCMEKStatus", reflect.TypeOf((*MockService)(nil).UpdateCMEKStatus), arg0, arg1, arg2)
}

// UpdateCluster mocks base method.
func (m *MockService) UpdateCluster(arg0 context.Context, arg1 string, arg2 *client.UpdateClusterSpecification, arg3 *client.UpdateClusterOptions) (*client.Cluster, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateCluster", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(*client.Cluster)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateCluster indicates an expected call of UpdateCluster.
func (mr *MockServiceMockRecorder) UpdateCluster(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateCluster", reflect.TypeOf((*MockService)(nil).UpdateCluster), arg0, arg1, arg2, arg3)
}

// UpdateSQLUserPassword mocks base method.
func (m *MockService) UpdateSQLUserPassword(arg0 context.Context, arg1, arg2 string, arg3 *client.UpdateSQLUserPasswordRequest) (*client.SQLUser, *http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateSQLUserPassword", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(*client.SQLUser)
	ret1, _ := ret[1].(*http.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// UpdateSQLUserPassword indicates an expected call of UpdateSQLUserPassword.
func (mr *MockServiceMockRecorder) UpdateSQLUserPassword(arg0, arg1, arg2, arg3 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateSQLUserPassword", reflect.TypeOf((*MockService)(nil).UpdateSQLUserPassword), arg0, arg1, arg2, arg3)
}