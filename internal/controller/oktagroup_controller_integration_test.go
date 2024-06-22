// oktagroupmanager_test.go
package controller

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/okta/okta-sdk-golang/v2/okta"
	"github.com/okta/okta-sdk-golang/v2/tests"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	accessmanagerv1 "github.com/franciscoprin/access-manager-operator/api/v1" // Adjust the import path
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	charSetAlphaLower = "abcdefghijklmnopqrstuvwxyz"
	charSetAlphaUpper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	charSetNumeric    = "0123456789"
	charSetSpecial    = "!@#$%&*"
	testPrefix        = "AMO_TEST_" // Access Manager Operator
	allCharSet        = charSetAlphaLower + charSetAlphaUpper + charSetNumeric + charSetSpecial
	passwordLength    = 10
	emailDomain       = "@example.com"
)

func randomString(charSet string, length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = charSet[rand.Intn(len(charSet))]
	}
	return string(result)
}

func generateRandomTestString() string {
	return testPrefix + randomString(charSetAlphaLower, 15)
}

func randomChar(charSet string) byte {
	return charSet[rand.Intn(len(charSet))]
}

func generateRandomOktaPassword() string {
	password := make([]byte, passwordLength)

	// Ensure the password meets the complexity requirements
	password[0] = randomChar(charSetAlphaLower)
	password[1] = randomChar(charSetAlphaUpper)
	password[2] = randomChar(charSetNumeric)
	password[3] = randomChar(charSetSpecial)

	// Fill the rest of the password length with random characters from all sets
	for i := 4; i < passwordLength; i++ {
		password[i] = randomChar(allCharSet)
	}

	return string(password)
}

func generateRandomEmail() string {
	return generateRandomTestString() + emailDomain
}

func createTestUser(ctx context.Context, oktaClient *okta.Client, t *testing.T) *okta.User {
	userCredentials := &okta.UserCredentials{
		Password: &okta.PasswordCredential{
			Value: generateRandomOktaPassword(),
		},
	}
	userProfile := okta.UserProfile{
		"firstName": "John",
		"lastName":  "Doe",
		"email":     generateRandomEmail(),
		"login":     generateRandomEmail(),
	}
	userRequest := &okta.CreateUserRequest{
		Credentials: userCredentials,
		Profile:     &userProfile,
	}
	user, _, err := oktaClient.User.CreateUser(ctx, *userRequest, nil)
	assert.NoError(t, err)

	return user
}

func removeTestUser(ctx context.Context, oktaClient *okta.Client, userID string) {
	if _, err := oktaClient.User.DeactivateUser(ctx, userID, nil); err != nil {
		log.Log.Error(err, "Unable to deactivate user")
	}
	if _, err := oktaClient.User.DeactivateOrDeleteUser(ctx, userID, nil); err != nil {
		log.Log.Error(err, "Unable to delete user")
	}
}

func configureLogger() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))
}

func initializeOktaClient(ctx context.Context, t *testing.T) (*okta.Client, context.Context) {
	ctx, oktaClient, err := tests.NewClient(ctx, okta.WithCache(false))
	assert.NoError(t, err)
	return oktaClient, ctx
}

func executeReconciler(ctx context.Context, t *testing.T, oktaGroupCRD *accessmanagerv1.OktaGroup) (client.Client, *OktaGroupReconciler, error) {
	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(oktaGroupCRD).WithStatusSubresource(oktaGroupCRD).Build()
	reconciler := &OktaGroupReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	res, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: oktaGroupCRD.Namespace,
			Name:      oktaGroupCRD.Name,
		},
	})
	assert.NoError(t, err)
	assert.False(t, res.Requeue)

	// Refresh the Okta group object
	err = fakeClient.Get(ctx, client.ObjectKey{Namespace: oktaGroupCRD.Namespace, Name: oktaGroupCRD.Name}, oktaGroupCRD)

	return fakeClient, reconciler, err
}

func extractOktaGroupCRDEmails(oktaUsersCRD []accessmanagerv1.OktaUserSpec) []string {
	emails := make([]string, len(oktaUsersCRD))
	for i, user := range oktaUsersCRD {
		emails[i] = user.Email
	}
	return emails
}

func removeOktaGroup(ctx context.Context, oktaClient *okta.Client, groupID string) {
	if _, err := oktaClient.Group.DeleteGroup(ctx, groupID); err != nil {
		log.Log.Info("Unable to delete Okta group", "groupID", groupID)
	}
}

func getGroupUserEmails(ctx context.Context, oktaClient *okta.Client, groupID string) ([]string, error) {
	groupUsersAPI, _, err := oktaClient.Group.ListGroupUsers(ctx, groupID, nil)
	if err != nil {
		return nil, err
	}

	groupUserEmails := make([]string, len(groupUsersAPI))
	for i, user := range groupUsersAPI {
		email, ok := (*user.Profile)["email"].(string)
		if !ok {
			return nil, errors.New("email not found in user profile")
		}
		groupUserEmails[i] = email
	}

	return groupUserEmails, nil
}

func createOktaUserSpecs(userEmails []string) []accessmanagerv1.OktaUserSpec {
	oktaUsers := make([]accessmanagerv1.OktaUserSpec, len(userEmails))
	for i, email := range userEmails {
		oktaUsers[i] = accessmanagerv1.OktaUserSpec{
			Email: email,
			Role:  "member",
		}
	}
	return oktaUsers
}

func TestOktaGroupReconciler_HappyPath(t *testing.T) {
	configureLogger()
	ctx := context.TODO()

	oktaClient, ctx := initializeOktaClient(ctx, t)

	user1 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user1.Id)

	user2 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user2.Id)

	accessmanagerv1.AddToScheme(scheme.Scheme)

	oktaGroup := &accessmanagerv1.OktaGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPrefix + "HappyPath",
		},
		Spec: accessmanagerv1.OktaGroupSpec{
			Description: "Test Okta Group",
			Users: []accessmanagerv1.OktaUserSpec{
				{
					Email: (*user1.Profile)["email"].(string),
					Role:  "member",
				},
				{
					Email: (*user2.Profile)["email"].(string),
					Role:  "member",
				},
			},
		},
	}

	// Call Reconcile to create the Okta group
	_, _, err := executeReconciler(ctx, t, oktaGroup)

	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)

	// Ensure the Okta group is deleted after test
	defer removeOktaGroup(ctx, oktaClient, group.Id)

	// Validate the Okta group CRD status
	assert.NoError(t, err)
	assert.Equal(t, oktaGroup.ObjectMeta.Name, group.Profile.Name)
	assert.Equal(t, oktaGroup.Spec.Description, group.Profile.Description)
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.Created.UTC()), metav1.NewTime(group.Created.UTC()))
	assert.Equal(t, oktaGroup.Status.Id, group.Id)
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.LastMembershipUpdated.UTC()), metav1.NewTime(group.LastMembershipUpdated.UTC()))
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.LastUpdated.UTC()), metav1.NewTime(group.LastUpdated.UTC()))

	// Ensure finalizers are added
	assert.Contains(t, oktaGroup.ObjectMeta.Finalizers, ConstOktaGroupFinalizer)

	// Validate the users were added to the Okta group
	groupUserEmails, err := getGroupUserEmails(ctx, oktaClient, group.Id)
	assert.NoError(t, err)
	assert.ElementsMatch(t, extractOktaGroupCRDEmails(oktaGroup.Spec.Users), groupUserEmails)

	// Trigger deletion by setting the DeletionTimestamp
	oktaGroup.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	fakeClient, _, _ := executeReconciler(ctx, t, oktaGroup)

	// Ensure that Okta group CRD was deleted
	oktaGroupCRDList := &accessmanagerv1.OktaGroupList{}
	err = fakeClient.List(ctx, oktaGroupCRDList)
	assert.NoError(t, err)
	assert.NotContains(t, oktaGroupCRDList.Items, *oktaGroup)

	// Ensure the Okta group is deleted
	_, _, err = oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	assert.Error(t, err)
}

func TestOktaGroupReconciler_UsersUpsert(t *testing.T) {
	configureLogger()
	ctx := context.TODO()

	oktaClient, ctx := initializeOktaClient(ctx, t)

	user1 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user1.Id)

	user2 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user2.Id)

	user3 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user3.Id)

	accessmanagerv1.AddToScheme(scheme.Scheme)

	oktaGroup := &accessmanagerv1.OktaGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPrefix + "UsersUpsert",
		},
		Spec: accessmanagerv1.OktaGroupSpec{
			Description: "Test Okta Group",
			Users: []accessmanagerv1.OktaUserSpec{
				{
					Email: (*user1.Profile)["email"].(string),
					Role:  "member",
				},
				{
					Email: (*user2.Profile)["email"].(string),
					Role:  "member",
				},
			},
		},
	}

	_, _, err := executeReconciler(ctx, t, oktaGroup)
	assert.NoError(t, err)

	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	defer removeOktaGroup(ctx, oktaClient, group.Id)

	assert.NoError(t, err)

	// Validate initial group users
	groupUserEmails, err := getGroupUserEmails(ctx, oktaClient, group.Id)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{(*user1.Profile)["email"].(string), (*user2.Profile)["email"].(string)}, groupUserEmails)

	// Update the group by adding user3 and removing user2
	oktaGroup.Spec.Users = []accessmanagerv1.OktaUserSpec{
		{
			Email: (*user1.Profile)["email"].(string),
			Role:  "member",
		},
		{
			Email: (*user3.Profile)["email"].(string),
			Role:  "member",
		},
	}

	_, _, err = executeReconciler(ctx, t, oktaGroup)
	assert.NoError(t, err)

	// Validate updated group users
	groupUserEmails, err = getGroupUserEmails(ctx, oktaClient, group.Id)
	assert.NoError(t, err)
	assert.ElementsMatch(t, extractOktaGroupCRDEmails(oktaGroup.Spec.Users), groupUserEmails)
	assert.ElementsMatch(t, []string{(*user1.Profile)["email"].(string), (*user3.Profile)["email"].(string)}, groupUserEmails)
}

func TestOktaGroupReconciler_IgnoreDisableUsers(t *testing.T) {
	configureLogger()
	ctx := context.TODO()

	oktaClient, ctx := initializeOktaClient(ctx, t)

	user1 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user1.Id)

	user2 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user2.Id)

	user3 := createTestUser(ctx, oktaClient, t)
	defer removeTestUser(ctx, oktaClient, user3.Id)

	// Deactivate user2 initially
	if _, err := oktaClient.User.DeactivateUser(ctx, user2.Id, nil); err != nil {
		t.Fatal(err)
	}

	accessmanagerv1.AddToScheme(scheme.Scheme)

	oktaGroup := &accessmanagerv1.OktaGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPrefix + "IgnoreDisabledUsers",
		},
		Spec: accessmanagerv1.OktaGroupSpec{
			Description: "Test Okta Group",
			Users: []accessmanagerv1.OktaUserSpec{
				{
					Email: (*user1.Profile)["email"].(string),
					Role:  "member",
				},
				{
					Email: (*user2.Profile)["email"].(string),
					Role:  "member",
				},
			},
		},
	}

	// Call Reconcile to create the Okta group with initial users
	_, _, err := executeReconciler(ctx, t, oktaGroup)

	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	defer removeOktaGroup(ctx, oktaClient, group.Id)
	assert.NoError(t, err)

	// Validate only active user is added to the group initially
	groupUserEmails, err := getGroupUserEmails(ctx, oktaClient, group.Id)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{(*user1.Profile)["email"].(string)}, groupUserEmails)

	// Deactivate user1 initially
	if _, err := oktaClient.User.DeactivateUser(ctx, user1.Id, nil); err != nil {
		t.Fatal(err)
	}

	oktaGroup.Spec.Users = []accessmanagerv1.OktaUserSpec{
		{
			Email: (*user1.Profile)["email"].(string),
			Role:  "member",
		},
		{
			Email: (*user2.Profile)["email"].(string),
			Role:  "member",
		},
		{
			Email: (*user3.Profile)["email"].(string),
			Role:  "member",
		},
	}

	// Call Reconcile to update the Okta group after user1 is deactivated and user3 is added
	_, _, err = executeReconciler(ctx, t, oktaGroup)
	assert.NoError(t, err)

	// Validate both users are added to the group
	groupUserEmails, err = getGroupUserEmails(ctx, oktaClient, group.Id)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{(*user3.Profile)["email"].(string)}, groupUserEmails)
}
