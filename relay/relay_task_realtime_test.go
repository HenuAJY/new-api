package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestSupportsRealtimeTaskFetchIncludesAPIMart(t *testing.T) {
	assert.True(t, supportsRealtimeTaskFetch(constant.ChannelTypeAPIMart))
	assert.True(t, supportsRealtimeTaskFetch(constant.ChannelTypeGemini))
	assert.True(t, supportsRealtimeTaskFetch(constant.ChannelTypeVertexAi))
	assert.False(t, supportsRealtimeTaskFetch(constant.ChannelTypeAicost))
}
