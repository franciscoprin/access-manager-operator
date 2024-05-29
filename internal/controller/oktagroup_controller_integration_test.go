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
)

// randStringFromCharSet generates a random string of 15 lower case letters
func randomTestString() string {
	result := make([]byte, 15)
	for i := 0; i < 15; i++ {
		result[i] = charSetAlphaLower[rand.Intn(len(charSetAlphaLower))]
	}
	return testPrefix + string(result)
}

func randomCharFromSet(charSet string) byte {
	index := rand.Intn(len(charSet))
	return charSet[index]
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
	return randomTestString() + "@example.com"
}

func TestOktaGroupReconciler(t *testing.T) {
	// Setup logger
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a Okta clients using the Okta SDK
	ctx, oktaClient, err := tests.NewClient(context.TODO(), okta.WithCache(false))

	// Create users
	uc := &okta.UserCredentials{
		Password: &okta.PasswordCredential{
			Value: randomOktaPassword(),
		},
	}
	profile := okta.UserProfile{}
	profile["firstName"] = "John"
	profile["lastName"] = "Activate"
	profile["email"] = randomEmail()
	profile["login"] = profile["email"]

	u := &okta.CreateUserRequest{
		Credentials: uc,
		Profile:     &profile,
	}
	user1, _, err := oktaClient.User.CreateUser(ctx, *u, nil)

	// Check if the user was created
	assert.NoError(t, err)

	// Delete the user after the test
	defer func() {
		_, err = oktaClient.User.DeactivateUser(ctx, user1.Id, nil)
		if err != nil {
			log.Log.Error(err, "unable to deactivate user")
		}
		_, err := oktaClient.User.DeactivateOrDeleteUser(ctx, user1.Id, nil)
		if err != nil {
			log.Log.Error(err, "unable to delete user")
		}
	}()

	// create another user
	profile["email"] = randomEmail()
	profile["login"] = profile["email"]
	u = &okta.CreateUserRequest{
		Credentials: uc,
		Profile:     &profile,
	}
	user2, _, err := oktaClient.User.CreateUser(ctx, *u, nil)

	// Delete the user after the test
	defer func() {
		_, err = oktaClient.User.DeactivateUser(ctx, user2.Id, nil)
		if err != nil {
			log.Log.Error(err, "unable to deactivate user")
		}
		_, err := oktaClient.User.DeactivateOrDeleteUser(ctx, user2.Id, nil)
		if err != nil {
			log.Log.Error(err, "unable to delete user")
		}
	}()

	// Register the OktaGroup type with the global scheme
	accessmanagerv1.AddToScheme(scheme.Scheme)

	// Create a fake client with a test OktaGroup object
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

	// Create a fake client
	// fakeClient := fake.NewClientBuilder().WithObjects(oktaGroup).Build()
	fakeClient := fake.NewClientBuilder().WithRuntimeObjects(oktaGroup).WithStatusSubresource(oktaGroup).Build()

	// Create reconciler with fake client
	reconciler := &OktaGroupReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	// Call Reconcile
	res, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: oktaGroup.Namespace,
			Name:      oktaGroup.Name,
		},
	})
	assert.NoError(t, err)
	assert.False(t, res.Requeue)

	// Refresh the Okta group CRD
	err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: oktaGroup.Namespace, Name: oktaGroup.Name}, oktaGroup)
	assert.NoError(t, err)

	// Get the okta group by ID
	group, _, err := oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	assert.NoError(t, err)

	// Check Okta group was added in the Okta group CRD status
	assert.Equal(t, oktaGroup.ObjectMeta.Name, group.Profile.Name)
	assert.Equal(t, oktaGroup.Spec.Description, group.Profile.Description)
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.Created.UTC()), metav1.NewTime(group.Created.UTC()))
	assert.Equal(t, oktaGroup.Status.Id, group.Id)
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.LastMembershipUpdated.UTC()), metav1.NewTime(group.LastMembershipUpdated.UTC()))
	assert.Equal(t, metav1.NewTime(oktaGroup.Status.LastUpdated.UTC()), metav1.NewTime(group.LastUpdated.UTC()))

	// Check if the users were added to the Okta group
	groupUsers, _, _ := oktaClient.Group.ListGroupUsers(ctx, group.Id, nil)

	// Convert the users to a slice of strings
	groupUsernames := make([]string, len(groupUsers))
	for i, user := range groupUsers {
		groupUsernames[i] = (*user.Profile)["email"].(string)
	}

	assert.ElementsMatch(t, oktaGroup.Spec.Users, groupUsernames)

	// Delete the Okta group by adding the ObjectMeta.DeletionTimestamp
	oktaGroup.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	// Add finalizer
	oktaGroup.ObjectMeta.Finalizers = []string{ConstOktaGroupFinalizer}

	fakeClient = fake.NewClientBuilder().WithRuntimeObjects(oktaGroup).WithStatusSubresource(oktaGroup).Build()
	assert.NoError(t, err)
	reconciler = &OktaGroupReconciler{
		Client: fakeClient,
		Scheme: scheme.Scheme,
	}

	// Call Reconcile
	res, err = reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: client.ObjectKey{
			Namespace: oktaGroup.Namespace,
			Name:      oktaGroup.Name,
		},
	})
	assert.Error(t, err)
	assert.False(t, res.Requeue)

	// Check if the Okta group was deleted
	group, _, err = oktaClient.Group.GetGroup(ctx, oktaGroup.Status.Id)
	assert.Nil(t, group)
	assert.Error(t, err)
}
