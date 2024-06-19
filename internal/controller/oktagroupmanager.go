package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/okta/okta-sdk-golang/v2/okta"
	"github.com/okta/okta-sdk-golang/v2/okta/query"
	"sigs.k8s.io/controller-runtime/pkg/log"

	accessmanagerv1 "github.com/franciscoprin/access-manager-operator/api/v1"
)

type OktaGroupManager struct {
	ctx          context.Context
	client       *okta.Client
	oktaGroupCRD *accessmanagerv1.OktaGroup
}

func NewOktaGroupManager(ctx context.Context, oktaGroupCRD *accessmanagerv1.OktaGroup, oktaClient *okta.Client) (*OktaGroupManager, error) {
	return &OktaGroupManager{
		ctx:          ctx,
		client:       oktaClient,
		oktaGroupCRD: oktaGroupCRD,
	}, nil
}

func (m *OktaGroupManager) UpsertOktaGroup() (*okta.Group, error) {
	groupProfile := &okta.GroupProfile{
		Name:        m.oktaGroupCRD.Name,
		Description: m.oktaGroupCRD.Spec.Description,
	}

	groupToUpsert := &okta.Group{
		Profile: groupProfile,
	}

	// Search for the group by name
	group, _ := m.SearchOktaGroup(m.oktaGroupCRD.Status.Id)

	// If the group is found, update it
	if group != nil {
		group, resp, err := m.client.Group.UpdateGroup(m.ctx, group.Id, *groupToUpsert)
		if err != nil {
			log.Log.Error(err, "unable to update Okta group")
			return nil, err
		}
		log.Log.Info("Updated Okta group", "group", group, "resp", resp)
		return group, nil
	}

	// If the group is not found, create it
	group, resp, err := m.client.Group.CreateGroup(m.ctx, *groupToUpsert)
	if err != nil {
		log.Log.Error(err, "unable to create Okta group")
		return nil, err
	}

	log.Log.Info("Created Okta group", "group", group, "resp", resp)

	return group, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (m *OktaGroupManager) searchUserByEmail(email string) (*okta.User, error) {
	filter := fmt.Sprintf(`profile.email eq "%s"`, email)
	queryParams := &query.Params{
		Filter: filter,
	}

	users, _, err := m.client.User.ListUsers(m.ctx, queryParams)
	if err != nil || len(users) == 0 {
		e := errors.New("User not found")
		log.Log.Error(err, e.Error(), "email", email)
		return nil, e
	}
	if len(users) > 1 {
		e := errors.New("more than one user found with that email")
		log.Log.Error(err, e.Error(), "email", email)
		return nil, e
	}
	return users[0], nil
}

func (m *OktaGroupManager) UpsertUsersToOktaGroup(group *okta.Group) error {
	if group == nil {
		return errors.New("group is nil")
	}

	oktaGroupUsersCRD := m.oktaGroupCRD.Spec.Users

	groupUsers, _, err := m.client.Group.ListGroupUsers(m.ctx, group.Id, nil)
	if err != nil {
		log.Log.Error(err, "unable to list group users")
		return err
	}

	oktaGroupUsers := make([]string, len(groupUsers))
	for i, user := range groupUsers {
		oktaGroupUsers[i] = (*user.Profile)["email"].(string)
	}

	// Add the users that are not in the Okta Group but were added to the Okta Group CRD
	for _, userEmailCRD := range oktaGroupUsersCRD {
		user, err := m.searchUserByEmail(userEmailCRD)
		if err != nil {
			continue
		}

		if contains(oktaGroupUsers, userEmailCRD) {
			log.Log.Info("User is already in Okta group", "user", user)
			continue
		}

		// Skip if the user is not active
		if user.Status != "ACTIVE" {
			log.Log.Info("User is not active", "user", user)
			continue
		}

		_, err = m.client.Group.AddUserToGroup(m.ctx, group.Id, user.Id)
		if err != nil {
			log.Log.Error(err, "unable to add user to Okta group")
			return err
		}
		log.Log.Info("Added user to Okta group", "group", group, "user", user)
	}

	// Remove the users that are in the Okta Group but were removed from the Okta Group CRD
	// Also remove those users that are not active
	for _, userEmail := range oktaGroupUsers {
		user, err := m.searchUserByEmail(userEmail)
		if err != nil {
			continue
		}

		if contains(oktaGroupUsersCRD, userEmail) && user.Status == "ACTIVE" {
			continue
		}

		_, err = m.client.Group.RemoveUserFromGroup(m.ctx, group.Id, user.Id)
		if err != nil {
			log.Log.Error(err, "unable to remove user from Okta group")
			return err
		}
		log.Log.Info("Removed user from Okta group", "group", group, "user", user)
	}

	return nil
}

func (m *OktaGroupManager) DeleteOktaGroup() error {
	// Search for the group by name
	group, err := m.SearchOktaGroup(m.oktaGroupCRD.Status.Id)

	// If there is an error, log it and return it
	if err != nil {
		log.Log.Error(err, "unable to search Okta group")
		return err
	}

	// If the group is found, delete it
	if group != nil {
		resp, err := m.client.Group.DeleteGroup(m.ctx, group.Id)
		if err != nil {
			log.Log.Error(err, "unable to delete Okta group")
			return err
		}

		log.Log.Info("Deleted Okta group", "group", group, "resp", resp)
	}

	return nil
}

func (m *OktaGroupManager) SearchOktaGroupByName() (*okta.Group, error) {
	// Search for the group by name
	groups, _, err := m.client.Group.ListGroups(m.ctx, &query.Params{Q: m.oktaGroupCRD.Name})
	if err != nil {
		log.Log.Error(err, "unable to list Okta groups")
		return nil, err
	}

	// If the group is found, return it
	for _, group := range groups {
		if group.Profile.Name == m.oktaGroupCRD.Name {
			return group, nil
		}
	}

	// If the group is not found, return an error
	return nil, errors.New("group not found")
}

func (m *OktaGroupManager) SearchOktaGroup(Id string) (*okta.Group, error) {
	if Id == "" {
		return nil, errors.New("Id is empty")
	}

	oktaGroupAPI, _, err := m.client.Group.GetGroup(m.ctx, Id)
	if err != nil {
		log.Log.Error(err, "unable to get OktaGroupAPI")
		return nil, err
	}

	return oktaGroupAPI, nil
}
