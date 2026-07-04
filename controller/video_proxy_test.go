package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCorsResponseHeader(t *testing.T) {
	assert.True(t, isCorsResponseHeader("Access-Control-Allow-Origin"))
	assert.True(t, isCorsResponseHeader("access-control-expose-headers"))
	assert.False(t, isCorsResponseHeader("Content-Type"))
	assert.False(t, isCorsResponseHeader("X-Request-Id"))
}
