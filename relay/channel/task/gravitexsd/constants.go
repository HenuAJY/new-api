package gravitexsd

const (
	ChannelName           = "gravitex-sd"
	defaultModelName      = "seedance-2-0"
	defaultBaseURL        = "https://api.gravitex.ai"
	defaultDuration       = 5
	managedAssetGroupName = "new-api-gravitex-sd"
	publicFailureMessage  = "video generation request failed"
)

var ModelList = []string{defaultModelName, "seedance-2-0-fast"}
