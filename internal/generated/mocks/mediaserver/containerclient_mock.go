// Code generated by mockery v2.52.2. DO NOT EDIT.

package mocks

import (
	context "context"

	container "git.netflux.io/rob/octoplex/internal/container"

	domain "git.netflux.io/rob/octoplex/internal/domain"

	mock "github.com/stretchr/testify/mock"
)

// ContainerClient is an autogenerated mock type for the ContainerClient type
type ContainerClient struct {
	mock.Mock
}

type ContainerClient_Expecter struct {
	mock *mock.Mock
}

func (_m *ContainerClient) EXPECT() *ContainerClient_Expecter {
	return &ContainerClient_Expecter{mock: &_m.Mock}
}

// ContainersWithLabels provides a mock function with given fields: _a0
func (_m *ContainerClient) ContainersWithLabels(_a0 map[string]string) container.LabelOptions {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for ContainersWithLabels")
	}

	var r0 container.LabelOptions
	if rf, ok := ret.Get(0).(func(map[string]string) container.LabelOptions); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(container.LabelOptions)
		}
	}

	return r0
}

// ContainerClient_ContainersWithLabels_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ContainersWithLabels'
type ContainerClient_ContainersWithLabels_Call struct {
	*mock.Call
}

// ContainersWithLabels is a helper method to define mock.On call
//   - _a0 map[string]string
func (_e *ContainerClient_Expecter) ContainersWithLabels(_a0 interface{}) *ContainerClient_ContainersWithLabels_Call {
	return &ContainerClient_ContainersWithLabels_Call{Call: _e.mock.On("ContainersWithLabels", _a0)}
}

func (_c *ContainerClient_ContainersWithLabels_Call) Run(run func(_a0 map[string]string)) *ContainerClient_ContainersWithLabels_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(map[string]string))
	})
	return _c
}

func (_c *ContainerClient_ContainersWithLabels_Call) Return(_a0 container.LabelOptions) *ContainerClient_ContainersWithLabels_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *ContainerClient_ContainersWithLabels_Call) RunAndReturn(run func(map[string]string) container.LabelOptions) *ContainerClient_ContainersWithLabels_Call {
	_c.Call.Return(run)
	return _c
}

// HasDockerNetwork provides a mock function with no fields
func (_m *ContainerClient) HasDockerNetwork() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for HasDockerNetwork")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// ContainerClient_HasDockerNetwork_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'HasDockerNetwork'
type ContainerClient_HasDockerNetwork_Call struct {
	*mock.Call
}

// HasDockerNetwork is a helper method to define mock.On call
func (_e *ContainerClient_Expecter) HasDockerNetwork() *ContainerClient_HasDockerNetwork_Call {
	return &ContainerClient_HasDockerNetwork_Call{Call: _e.mock.On("HasDockerNetwork")}
}

func (_c *ContainerClient_HasDockerNetwork_Call) Run(run func()) *ContainerClient_HasDockerNetwork_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *ContainerClient_HasDockerNetwork_Call) Return(_a0 bool) *ContainerClient_HasDockerNetwork_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *ContainerClient_HasDockerNetwork_Call) RunAndReturn(run func() bool) *ContainerClient_HasDockerNetwork_Call {
	_c.Call.Return(run)
	return _c
}

// RemoveContainers provides a mock function with given fields: _a0, _a1
func (_m *ContainerClient) RemoveContainers(_a0 context.Context, _a1 container.LabelOptions) error {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for RemoveContainers")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, container.LabelOptions) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ContainerClient_RemoveContainers_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RemoveContainers'
type ContainerClient_RemoveContainers_Call struct {
	*mock.Call
}

// RemoveContainers is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 container.LabelOptions
func (_e *ContainerClient_Expecter) RemoveContainers(_a0 interface{}, _a1 interface{}) *ContainerClient_RemoveContainers_Call {
	return &ContainerClient_RemoveContainers_Call{Call: _e.mock.On("RemoveContainers", _a0, _a1)}
}

func (_c *ContainerClient_RemoveContainers_Call) Run(run func(_a0 context.Context, _a1 container.LabelOptions)) *ContainerClient_RemoveContainers_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(container.LabelOptions))
	})
	return _c
}

func (_c *ContainerClient_RemoveContainers_Call) Return(_a0 error) *ContainerClient_RemoveContainers_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *ContainerClient_RemoveContainers_Call) RunAndReturn(run func(context.Context, container.LabelOptions) error) *ContainerClient_RemoveContainers_Call {
	_c.Call.Return(run)
	return _c
}

// RunContainer provides a mock function with given fields: _a0, _a1
func (_m *ContainerClient) RunContainer(_a0 context.Context, _a1 container.RunContainerParams) (<-chan domain.Container, <-chan error) {
	ret := _m.Called(_a0, _a1)

	if len(ret) == 0 {
		panic("no return value specified for RunContainer")
	}

	var r0 <-chan domain.Container
	var r1 <-chan error
	if rf, ok := ret.Get(0).(func(context.Context, container.RunContainerParams) (<-chan domain.Container, <-chan error)); ok {
		return rf(_a0, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, container.RunContainerParams) <-chan domain.Container); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan domain.Container)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, container.RunContainerParams) <-chan error); ok {
		r1 = rf(_a0, _a1)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(<-chan error)
		}
	}

	return r0, r1
}

// ContainerClient_RunContainer_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RunContainer'
type ContainerClient_RunContainer_Call struct {
	*mock.Call
}

// RunContainer is a helper method to define mock.On call
//   - _a0 context.Context
//   - _a1 container.RunContainerParams
func (_e *ContainerClient_Expecter) RunContainer(_a0 interface{}, _a1 interface{}) *ContainerClient_RunContainer_Call {
	return &ContainerClient_RunContainer_Call{Call: _e.mock.On("RunContainer", _a0, _a1)}
}

func (_c *ContainerClient_RunContainer_Call) Run(run func(_a0 context.Context, _a1 container.RunContainerParams)) *ContainerClient_RunContainer_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(container.RunContainerParams))
	})
	return _c
}

func (_c *ContainerClient_RunContainer_Call) Return(_a0 <-chan domain.Container, _a1 <-chan error) *ContainerClient_RunContainer_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *ContainerClient_RunContainer_Call) RunAndReturn(run func(context.Context, container.RunContainerParams) (<-chan domain.Container, <-chan error)) *ContainerClient_RunContainer_Call {
	_c.Call.Return(run)
	return _c
}

// NewContainerClient creates a new instance of ContainerClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewContainerClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *ContainerClient {
	mock := &ContainerClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
