package chat

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ModelsTopicToAPI converts a models.ConversationTopic to an api.ConversationTopic.
// Returns nil if the input is nil or has no topic_id set.
func ModelsTopicToAPI(topic *models.ConversationTopic) *api.ConversationTopic {
	if topic == nil {
		return nil
	}
	switch t := topic.GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		return &api.ConversationTopic{
			TopicId: &api.ConversationTopic_TransferId{TransferId: t.TransferId},
		}
	case *models.ConversationTopic_RequestId:
		return &api.ConversationTopic{
			TopicId: &api.ConversationTopic_RequestId{RequestId: t.RequestId},
		}
	case *models.ConversationTopic_ExperienceId:
		return &api.ConversationTopic{
			TopicId: &api.ConversationTopic_ExperienceId{ExperienceId: t.ExperienceId},
		}
	case *models.ConversationTopic_GearId:
		return &api.ConversationTopic{
			TopicId: &api.ConversationTopic_GearId{GearId: t.GearId},
		}
	case *models.ConversationTopic_CommunityId:
		return &api.ConversationTopic{
			TopicId: &api.ConversationTopic_CommunityId{CommunityId: t.CommunityId},
		}
	default:
		return nil
	}
}

// APITopicToModels converts an api.ConversationTopic to a models.ConversationTopic.
// Returns nil if the input is nil or has no topic_id set.
func APITopicToModels(topic *api.ConversationTopic) *models.ConversationTopic {
	if topic == nil {
		return nil
	}
	switch t := topic.GetTopicId().(type) {
	case *api.ConversationTopic_TransferId:
		return &models.ConversationTopic{
			TopicId: &models.ConversationTopic_TransferId{TransferId: t.TransferId},
		}
	case *api.ConversationTopic_RequestId:
		return &models.ConversationTopic{
			TopicId: &models.ConversationTopic_RequestId{RequestId: t.RequestId},
		}
	case *api.ConversationTopic_ExperienceId:
		return &models.ConversationTopic{
			TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: t.ExperienceId},
		}
	case *api.ConversationTopic_GearId:
		return &models.ConversationTopic{
			TopicId: &models.ConversationTopic_GearId{GearId: t.GearId},
		}
	case *api.ConversationTopic_CommunityId:
		return &models.ConversationTopic{
			TopicId: &models.ConversationTopic_CommunityId{CommunityId: t.CommunityId},
		}
	default:
		return nil
	}
}
