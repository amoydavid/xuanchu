package app

import (
	"git.dajee.net/dajee/xuanchu/internal/storage"
	"git.dajee.net/dajee/xuanchu/internal/task"
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

	rows, err := s.userRepo.ListByIDs(unique)
	if err != nil {
		return nil, err
	}
	users := make(map[string]storage.User, len(rows))
	for _, user := range rows {
		users[user.ID] = user
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
				DisplayName: user.DisplayName,
				Email:       user.Email,
				ExternalIDs: extByUser[user.ID],
			}
		} else {
			result[id] = task.UserInfo{ID: id, Name: id}
		}
	}
	return result, nil
}
