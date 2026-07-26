package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomChannelTypeUsesReservedRangeWithoutBaseURLIndexing(t *testing.T) {
	assert.Equal(t, 9801, ChannelTypeMoyinAI)
	assert.Equal(t, 9802, ChannelTypeViraldance933m)
	assert.Equal(t, 9803, ChannelTypeViraldance933x)
	assert.Equal(t, 9804, ChannelTypeViraldance933v)
	assert.Equal(t, 9805, ChannelTypeViraldance431Fast)
	assert.Equal(t, 9806, ChannelTypeGravitexSD)
	assert.Empty(t, GetChannelBaseURL(ChannelTypeMoyinAI))
	assert.Empty(t, GetChannelBaseURL(ChannelTypeViraldance933m))
	assert.Empty(t, GetChannelBaseURL(ChannelTypeViraldance933x))
	assert.Empty(t, GetChannelBaseURL(ChannelTypeViraldance933v))
	assert.Equal(t, "https://api.openai.com", GetChannelBaseURL(ChannelTypeOpenAI))
	assert.Empty(t, GetChannelBaseURL(-1))
}
