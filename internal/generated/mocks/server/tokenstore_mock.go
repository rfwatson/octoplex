// Code generated by mockery v2.52.2. DO NOT EDIT.

package mocks

import (
	domain "git.netflux.io/rob/octoplex/internal/domain"
	mock "github.com/stretchr/testify/mock"
)

// TokenStore is an autogenerated mock type for the TokenStore type
type TokenStore struct {
	mock.Mock
}

type TokenStore_Expecter struct {
	mock *mock.Mock
}

func (_m *TokenStore) EXPECT() *TokenStore_Expecter {
	return &TokenStore_Expecter{mock: &_m.Mock}
}

// Delete provides a mock function with given fields: key
func (_m *TokenStore) Delete(key string) error {
	ret := _m.Called(key)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(key)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TokenStore_Delete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Delete'
type TokenStore_Delete_Call struct {
	*mock.Call
}

// Delete is a helper method to define mock.On call
//   - key string
func (_e *TokenStore_Expecter) Delete(key interface{}) *TokenStore_Delete_Call {
	return &TokenStore_Delete_Call{Call: _e.mock.On("Delete", key)}
}

func (_c *TokenStore_Delete_Call) Run(run func(key string)) *TokenStore_Delete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string))
	})
	return _c
}

func (_c *TokenStore_Delete_Call) Return(_a0 error) *TokenStore_Delete_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *TokenStore_Delete_Call) RunAndReturn(run func(string) error) *TokenStore_Delete_Call {
	_c.Call.Return(run)
	return _c
}

// Get provides a mock function with given fields: key
func (_m *TokenStore) Get(key string) (domain.Token, error) {
	ret := _m.Called(key)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 domain.Token
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (domain.Token, error)); ok {
		return rf(key)
	}
	if rf, ok := ret.Get(0).(func(string) domain.Token); ok {
		r0 = rf(key)
	} else {
		r0 = ret.Get(0).(domain.Token)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// TokenStore_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type TokenStore_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - key string
func (_e *TokenStore_Expecter) Get(key interface{}) *TokenStore_Get_Call {
	return &TokenStore_Get_Call{Call: _e.mock.On("Get", key)}
}

func (_c *TokenStore_Get_Call) Run(run func(key string)) *TokenStore_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string))
	})
	return _c
}

func (_c *TokenStore_Get_Call) Return(_a0 domain.Token, _a1 error) *TokenStore_Get_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *TokenStore_Get_Call) RunAndReturn(run func(string) (domain.Token, error)) *TokenStore_Get_Call {
	_c.Call.Return(run)
	return _c
}

// Put provides a mock function with given fields: key, value
func (_m *TokenStore) Put(key string, value domain.Token) error {
	ret := _m.Called(key, value)

	if len(ret) == 0 {
		panic("no return value specified for Put")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(string, domain.Token) error); ok {
		r0 = rf(key, value)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TokenStore_Put_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Put'
type TokenStore_Put_Call struct {
	*mock.Call
}

// Put is a helper method to define mock.On call
//   - key string
//   - value domain.Token
func (_e *TokenStore_Expecter) Put(key interface{}, value interface{}) *TokenStore_Put_Call {
	return &TokenStore_Put_Call{Call: _e.mock.On("Put", key, value)}
}

func (_c *TokenStore_Put_Call) Run(run func(key string, value domain.Token)) *TokenStore_Put_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(domain.Token))
	})
	return _c
}

func (_c *TokenStore_Put_Call) Return(_a0 error) *TokenStore_Put_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *TokenStore_Put_Call) RunAndReturn(run func(string, domain.Token) error) *TokenStore_Put_Call {
	_c.Call.Return(run)
	return _c
}

// NewTokenStore creates a new instance of TokenStore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewTokenStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *TokenStore {
	mock := &TokenStore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
