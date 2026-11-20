package domain

type ProfilePrivacy struct {
	ProfileVisibility     string `json:"profile_visibility"`
	DescriptionVisibility string `json:"description_visibility"`
	FriendsVisibility     string `json:"friends_visibility"`
	CommunitiesVisibility string `json:"communities_visibility"`
}
