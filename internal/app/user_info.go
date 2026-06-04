package app

import (
	"github.com/dajee/taskg/internal/storage/sqlite"
	"github.com/dajee/taskg/internal/task"
)

func (s *Service) resolveUserInfos(ids []string) (map[string]task.UserInfo, error) {
	seen := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return map[string]task.UserInfo{}, nil
	}

	users := make(map[string]sqlite.User, len(unique))
	for _, id := range unique {
		user, err := s.userRepo.GetByID(id)
		if err == sqlite.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		users[id] = user
	}

	extByUser, err := s.loadExternalIDsByUsers(unique)
	if err != nil {
		return nil, err
	}

	result := make(map[string]task.UserInfo, len(unique))
	for _, id := range unique {
		if user, ok := users[id]; ok {
			result[id] = task.UserInfo{
				ID:          user.ID,
				Name:        user.Name,
				Email:       user.Email,
				ExternalIDs: extByUser[user.ID],
			}
		} else {
			result[id] = task.UserInfo{ID: id, Name: id}
		}
	}
	return result, nil
}
