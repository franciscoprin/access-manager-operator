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

func TestOktaGroupReconciler(t *testing.T) {
	setupLogger()
	ctx := context.TODO()

	oktaClient, ctx := setupOktaClient(ctx, t)

	user1 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user1.Id)

	user2 := createUser(ctx, oktaClient, t)
	defer deleteUser(ctx, oktaClient, user2.Id)

	accessmanagerv1.AddToScheme(scheme.Scheme)

	oktaGroup := &accessmanagerv1.OktaGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPrefix + "okta-group",
		},
		Spec: accessmanagerv1.OktaGroupSpec{
			Description: "Test Okta Group",
			Users: []string{
				(*user1.Profile)["email"].(string),
				(*user2.Profile)["email"].(string),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(oktaGroup).WithStatusSubresource(oktaGroup).Build()

	reconciler := &OktaGroupReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	// Call Reconcile to create the Okta group
	res, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: oktaGroup.Namespace,
			Name:      oktaGroup.Name,
		},
	})
	assert.NoError(t, err)
	assert.False(t, res.Requeue)

	// Refresh the Okta group object
	err = fakeClient.Get(ctx, client.ObjectKey{Namespace: oktaGroup.Namespace, Name: oktaGroup.Name}, oktaGroup)
	assert.NoError(t, err)

	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	assert.NoError(t, err)

	// Check that the Okta group CRD status matches the Okta group
	assert.Equal(t, oktaGroup.ObjectMeta.Name, group.Profile.Name)
	assert.Equal(t, oktaGroup.Spec.Description, group.Profile.Description)
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.Created.UTC()), metav1.NewTime(group.Created.UTC()))
	assert.Equal(t, oktaGroup.Status.Id, group.Id)
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.LastMembershipUpdated.UTC()), metav1.NewTime(group.LastMembershipUpdated.UTC()))
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.LastUpdated.UTC()), metav1.NewTime(group.LastUpdated.UTC()))

	// Check that the users were added to the Okta group
	groupUsers, _, err := oktaClient.Group.ListGroupUsers(ctx, group.Id, nil)
	assert.NoError(t, err)

	groupUsernames := make([]string, len(groupUsers))
	for i, user := range groupUsers {
		groupUsernames[i] = (*user.Profile)["email"].(string)
	}
	assert.ElementsMatch(t, oktaGroup.Spec.Users, groupUsernames)

	// Delete the Okta group by adding the ObjectMeta.DeletionTimestamp and the finalizer
	oktaGroup.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	oktaGroup.ObjectMeta.Finalizers = []string{ConstOktaGroupFinalizer}

	fakeClient = fake.NewClientBuilder().WithRuntimeObjects(oktaGroup).WithStatusSubresource(oktaGroup).Build()
	reconciler = &OktaGroupReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	res, err = reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: oktaGroup.Namespace,
			Name:      oktaGroup.Name,
		},
	})
	assert.Error(t, err)
	assert.False(t, res.Requeue)

	// Check that the Okta group was deleted
	group, _, err = oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	assert.Nil(t, group)
	assert.Error(t, err)
}
