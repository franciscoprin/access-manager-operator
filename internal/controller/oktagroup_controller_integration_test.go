// oktagroupmanager_test.go
package controller

import (
	"context"
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
	testPrefix        = "AMO_TEST_"
	allCharSet        = charSetAlphaLower + charSetAlphaUpper + charSetNumeric + charSetSpecial
	passwordLength    = 10
	emailDomain       = "@example.com"
)

func randomStringFromCharSet(charSet string, length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = charSet[rand.Intn(len(charSet))]
	}
	return string(result)
}

func randomTestString() string {
	return testPrefix + randomStringFromCharSet(charSetAlphaLower, 15)
}

func randomCharFromSet(charSet string) byte {
	return charSet[rand.Intn(len(charSet))]
}

func randomOktaPassword() string {
	passwordLength := 10
	password := make([]byte, passwordLength)

	// Ensure the password meets the complexity requirements
	password[0] = randomCharFromSet(charSetAlphaLower)
	password[1] = randomCharFromSet(charSetAlphaUpper)
	password[2] = randomCharFromSet(charSetNumeric)
	password[3] = randomCharFromSet(charSetSpecial)

	// Fill the rest of the password length with random characters from all sets
	for i := 4; i < passwordLength; i++ {
		password[i] = randomCharFromSet(allCharSet)
	}

	return string(password)
}

func randomEmail() string {
	return randomTestString() + emailDomain
}

func createUser(ctx context.Context, oktaClient *okta.Client, t *testing.T) *okta.User {
	uc := &okta.UserCredentials{
		Password: &okta.PasswordCredential{
			Value: randomOktaPassword(),
		},
	}
	profile := okta.UserProfile{
		"firstName": "John",
		"lastName":  "Doe",
		"email":     randomEmail(),
		"login":     randomEmail(),
	}
	u := &okta.CreateUserRequest{
		Credentials: uc,
		Profile:     &profile,
	}
	user, _, err := oktaClient.User.CreateUser(ctx, *u, nil)
	assert.NoError(t, err)

	return user
}

func deleteUser(ctx context.Context, oktaClient *okta.Client, userID string) {
	if _, err := oktaClient.User.DeactivateUser(ctx, userID, nil); err != nil {
		log.Log.Error(err, "unable to deactivate user")
	}
	if _, err := oktaClient.User.DeactivateOrDeleteUser(ctx, userID, nil); err != nil {
		log.Log.Error(err, "unable to delete user")
	}
}

func setupLogger() {
	log.SetLogger(zap.New(zap.UseDevMode(true)))
}

func setupOktaClient(ctx context.Context, t *testing.T) (*okta.Client, context.Context) {
	ctx, oktaClient, err := tests.NewClient(ctx, okta.WithCache(false))
	assert.NoError(t, err)
	return oktaClient, ctx
}

func callReconciler(ctx context.Context, t *testing.T, oktaGroupCRD *accessmanagerv1.OktaGroup) (client.Client, *OktaGroupReconciler, error) {

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

func TestOktaGroupReconciler_HappyPath(t *testing.T) {
	setupLogger()
	ctx := context.TODO()

	oktaClient, ctx := setupOktaClient(ctx, t)

	user1 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user1.Id)

	user2 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user2.Id)

	accessmanagerv1.AddToScheme(scheme.Scheme)

	oktaGroupCRD := &accessmanagerv1.OktaGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPrefix + "HappyPath",
		},
		Spec: accessmanagerv1.OktaGroupSpec{
			Description: "Test Okta Group",
			Users: []string{
				(*user1.Profile)["email"].(string),
				(*user2.Profile)["email"].(string),
			},
		},
	}

	// Call Reconcile to create the Okta group
	_, _, err := callReconciler(ctx, t, oktaGroupCRD)

	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroupCRD.Status.Id)
	assert.NoError(t, err)

	// Check that the Okta group CRD status matches the Okta group
	assert.Equal(t, oktaGroupCRD.ObjectMeta.Name, group.Profile.Name)
	assert.Equal(t, oktaGroupCRD.Spec.Description, group.Profile.Description)
	assert.Equal(t, metav1.NewTime(oktaGroupCRD.Status.Created.UTC()), metav1.NewTime(group.Created.UTC()))
	assert.Equal(t, oktaGroupCRD.Status.Id, group.Id)
	assert.Equal(t, metav1.NewTime(oktaGroupCRD.Status.LastMembershipUpdated.UTC()), metav1.NewTime(group.LastMembershipUpdated.UTC()))
	assert.Equal(t, metav1.NewTime(oktaGroupCRD.Status.LastUpdated.UTC()), metav1.NewTime(group.LastUpdated.UTC()))

	// Check that finalizers were added
	assert.Contains(t, oktaGroupCRD.ObjectMeta.Finalizers, ConstOktaGroupFinalizer)

	// Check that the users were added to the Okta group
	groupUsersAPI, _, err := oktaClient.Group.ListGroupUsers(ctx, group.Id, nil)
	assert.NoError(t, err)

	groupUsersEmailAPI := make([]string, len(groupUsersAPI))
	for i, user := range groupUsersAPI {
		groupUsersEmailAPI[i] = (*user.Profile)["email"].(string)
	}
	assert.ElementsMatch(t, oktaGroupCRD.Spec.Users, groupUsersEmailAPI)

	// Set the DeletionTimestamp to trigger deletion
	oktaGroupCRD.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	// Reconciliation for deletion
	fakeClient, _, _ := callReconciler(ctx, t, oktaGroupCRD)

	// Check that the OktaGroupCRD was deleted
	//// List the oktaGroupCRDs
	oktaGroupCRDList := &accessmanagerv1.OktaGroupList{}
	err = fakeClient.List(ctx, oktaGroupCRDList)
	//// Check that the oktaGroupCRD is not in the list
	assert.NoError(t, err)
	assert.NotContains(t, oktaGroupCRDList.Items, *oktaGroupCRD)

	// Check that the Okta group was deleted
	group, _, err = oktaClient.Group.GetGroup(ctx, oktaGroupCRD.Status.Id)
	assert.Nil(t, group)
	assert.Error(t, err)
}

func TestOktaGroupReconciler_UsersUpsert(t *testing.T) {
	setupLogger()
	ctx := context.TODO()
	accessmanagerv1.AddToScheme(scheme.Scheme)

	oktaClient, ctx := setupOktaClient(ctx, t)

	user1 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user1.Id)

	user2 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user2.Id)

	user3 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user3.Id)

	oktaGroupCRD := &accessmanagerv1.OktaGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPrefix + "UsersUpsert",
		},
		Spec: accessmanagerv1.OktaGroupSpec{
			Description: "Test Okta Group",
			Users: []string{
				(*user1.Profile)["email"].(string),
				(*user2.Profile)["email"].(string),
				(*user3.Profile)["email"].(string),
			},
		},
	}

	// Call Reconcile to create the Okta group
	_, _, err := callReconciler(ctx, t, oktaGroupCRD)
	assert.NoError(t, err)

	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroupCRD.Status.Id)
	assert.NoError(t, err)

	// Check that the users were added to the Okta group
	groupUsers, _, err := oktaClient.Group.ListGroupUsers(ctx, group.Id, nil)
	assert.NoError(t, err)

	groupUsernames := make([]string, len(groupUsers))
	for i, user := range groupUsers {
		groupUsernames[i] = (*user.Profile)["email"].(string)
	}
	assert.ElementsMatch(t, oktaGroupCRD.Spec.Users, groupUsernames)

	// Remove a user from the Okta group CRD

	// Remove user2 from the Okta group
	expectedUsers := []string{
		(*user1.Profile)["email"].(string),
		// (*user2.Profile)["email"].(string), // Remove user2
		(*user3.Profile)["email"].(string),
	}

	oktaGroupCRD.Spec.Users = expectedUsers

	// Reconciliation to remove user2
	_, _, err = callReconciler(ctx, t, oktaGroupCRD)
	assert.NoError(t, err)

	// Check that the user was remove from the Okta group
	groupUsers, _, err = oktaClient.Group.ListGroupUsers(ctx, group.Id, nil)
	assert.NoError(t, err)

	groupUsernames = make([]string, len(groupUsers))
	for i, user := range groupUsers {
		groupUsernames[i] = (*user.Profile)["email"].(string)
	}
	assert.ElementsMatch(t, expectedUsers, groupUsernames)
	// Check that the removed user is not in the Okta group
	assert.NotContains(t, groupUsernames, (*user2.Profile)["email"].(string))

	// Add back the removed user to the Okta group CRD
	expectedUsers = []string{
		(*user1.Profile)["email"].(string),
		(*user2.Profile)["email"].(string), // Add back user2
		(*user3.Profile)["email"].(string),
	}

	oktaGroupCRD.Spec.Users = expectedUsers

	// Reconciliation to add back user2
	_, _, err = callReconciler(ctx, t, oktaGroupCRD)
	assert.NoError(t, err)

	// Check that the user was remove from the Okta group
	groupUsers, _, err = oktaClient.Group.ListGroupUsers(ctx, group.Id, nil)
	assert.NoError(t, err)

	groupUsernames = make([]string, len(groupUsers))
	for i, user := range groupUsers {
		groupUsernames[i] = (*user.Profile)["email"].(string)
	}
	assert.ElementsMatch(t, expectedUsers, groupUsernames)

	// Check that the removed user is not in the Okta group
	assert.Contains(t, groupUsernames, (*user2.Profile)["email"].(string))

	// Set the DeletionTimestamp to trigger deletion
	oktaGroupCRD.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	// Reconciliation for deletion
	fakeClient, _, _ := callReconciler(ctx, t, oktaGroupCRD)

	// Check that the OktaGroupCRD was deleted
	//// List the oktaGroupCRDs
	oktaGroupCRDList := &accessmanagerv1.OktaGroupList{}
	err = fakeClient.List(ctx, oktaGroupCRDList)
	//// Check that the oktaGroupCRD is not in the list
	assert.NoError(t, err)
	assert.NotContains(t, oktaGroupCRDList.Items, *oktaGroupCRD)

	// Check that the Okta group was deleted
	group, _, err = oktaClient.Group.GetGroup(ctx, oktaGroupCRD.Status.Id)
	assert.Nil(t, group)
	assert.Error(t, err)
}
