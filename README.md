# access-manager-operator
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Set Okta credentials as secrets
1. Create a file named okta-secrets.yaml with the following content. Replace the placeholder values with your actual Okta domain and API token encoded in Base64.

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
        name: okta-secrets
        namespace: access-manager-operator
        type: Opaque
    data:
        client-org-url: <base64_encoded_okta_domain>
        client-token: <base64_encoded_api_token>
    ```

2. To encode your Okta domain and API token in Base64, you can use the following command:

    ```bash
    echo -n 'your_okta_domain' | base64 # <base64_encoded_okta_domain>
    echo -n 'your_api_token' | base64 # <base64_encoded_api_token>
    ```

    This will output the Base64 encoded values which you can then use in the YAML file.

3. Use kubectl to apply the YAML file:

    ```bash
    kubectl apply -f okta-secrets.yaml
    ```

4. Verify if the secrets have been created successfully, the secrets can described using the following command:

    ```bash
    kubectl describe secret okta-secrets -n access-manager-operator
    ```

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -k config/samples/
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/access-manager-operator:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/access-manager-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

