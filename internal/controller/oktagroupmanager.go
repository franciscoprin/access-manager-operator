package controller

import (
	"context"
	"errors"

	"github.com/okta/okta-sdk-golang/v2/okta"
	"github.com/okta/okta-sdk-golang/v2/okta/query"
	"sigs.k8s.io/controller-runtime/pkg/log"

	accessmanagerv1 "github.com/franciscoprin/access-manager-operator/api/v1"
)

type OktaGroupManager struct {
	ctx       context.Context
	client    *okta.Client
	oktaGroup *accessmanagerv1.OktaGroup
}

func NewOktaGroupManager(ctx context.Context, oktaGroup *accessmanagerv1.OktaGroup, oktaClient *okta.Client) (*OktaGroupManager, error) {
	return &OktaGroupManager{
		ctx:       ctx,
		client:    oktaClient,
		oktaGroup: oktaGroup,
	}, nil
}

func (m *OktaGroupManager) UpsertOktaGroup() (*okta.Group, error) {
	groupProfile := &okta.GroupProfile{
		Name:        m.oktaGroup.Name,
		Description: m.oktaGroup.Spec.Description,
	}

	groupToUpsert := &okta.Group{
		Profile: groupProfile,
	}

	// Search for the group by name
	group, _ := m.SearchOktaGroup()

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

func (m *OktaGroupManager) UpsertUsersToOktaGroup(group *okta.Group) error {
	// Check that group is not nil
	if group == nil {
		return errors.New("group is nil")
	}

	// Add the users to the Okta group
	for _, userEmail := range m.oktaGroup.Spec.Users {
		// Search for the user by email
		user, _, err := m.client.User.GetUser(m.ctx, userEmail)

		// If there isn't a user with that email, log it and continue
		if err != nil {
			log.Log.Info("User not found", "email", userEmail)
			continue
		}

		// Add the user to the group
		_, err = m.client.Group.AddUserToGroup(m.ctx, group.Id, user.Id)
		if err != nil {
			log.Log.Error(err, "unable to add user to Okta group")
			return err
		}

		log.Log.Info("Added user to Okta group", "group", group, "user", user)
	}

	// Refresh the group
	group, _ = m.SearchOktaGroup()

	return nil
}

func (m *OktaGroupManager) DeleteOktaGroup() error {
	// Search for the group by name
	group, err := m.SearchOktaGroup()

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

func (m *OktaGroupManager) SearchOktaGroup() (*okta.Group, error) {
	// Search for the group by name
	groups, _, err := m.client.Group.ListGroups(m.ctx, &query.Params{Q: m.oktaGroup.Name})
	if err != nil {
		log.Log.Error(err, "unable to list Okta groups")
		return nil, err
	}

	// If the group is found, return it
	for _, group := range groups {
		if group.Profile.Name == m.oktaGroup.Name {
			return group, nil
		}
	}

	// If the group is not found, return an error
	return nil, errors.New("group not found")
}
