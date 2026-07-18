package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomChannelTypeUsesReservedRangeWithoutBaseURLIndexing(t *testing.T) {
	assert.Equal(t, 9801, ChannelTypeMoyinAI)
	assert.Empty(t, GetChannelBaseURL(ChannelTypeMoyinAI))
	assert.Equal(t, "https://api.openai.com", GetChannelBaseURL(ChannelTypeOpenAI))
	assert.Empty(t, GetChannelBaseURL(-1))
}
