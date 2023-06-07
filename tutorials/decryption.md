# Image Decryption Support

This document describes the method to configure image decryption support.
Please note that this is still an experimental feature.

## Encrypted Container Images

Encrypted container images are OCI images that contain encrypted blobs.
An example of how these encrypted images can be created through the use of
[containers/skopeo](https://github.com/containers/skopeo/blob/master/docs/skopeo-copy.1.md).
To decrypt these images, `CRI-O` needs to have access to the
corresponding private key(s).

## Key Models

Encryption ties trust to an entity based on the model in which a key is
associated with it. We call this the key model. There are two currently supported
key models in which encrypted containers can be used.
These are based on two main use cases.

1. **Node** Key Model - In this model encryption is tied to workers. The use case
   here revolves around the idea that an image should be only decryptable on the
   trusted host. Although the granularity of access is more relaxed (per node),
   it is beneficial because of the various node based technologies that help bootstrap
   trust in worker nodes and perform secure key distribution (i.e. TPM, host attestation,
   secure/measured boot). In this scenario, runtimes are capable of fetching
   the necessary decryption keys.

2. **Multitenant** Key Model -
   **This model is not yet supported by CRI-O, but will be in the future.**
   In this model,the trust of encryption is tied to the cluster or users within
   a cluster. This allows multi-tenancy of users and is useful in the case where
   multiple users of Kubernetes each want to bring their encrypted images.
   This is based on the [KEP](https://github.com/kubernetes/enhancements/pull/1066)
   that introduces `ImageDecryptSecrets`.

## Configuring Image Decryption for **Node** key model

In order to set up image decryption support, add an overwrite to
`/etc/crio/crio.conf.d/01-decrypt.conf` as follows:

```toml
[crio.runtime]
# decryption_keys_path is the path where the keys required for
# image decryption are located
decryption_keys_path = "/etc/crio/keys/"

```

`decryption_keys_path` should be the path where `CRI-O` can find the keys
required for image decryption.

After modifying this config, you need to restart the `CRI-O` service.

Alternatively, if you are starting the `CRI-O` from the command line, the argument
`--decryption-keys-path` can be provided pointing to the folder that
contains required decryption keys.

## Verification of the Decryption Capabilities

Although the latest master branch of the `docker/distribution` registry supports
encrypted images, many popular public registries such as Docker hub or `quay.io`
don't support encrypted images yet.

For the easy verification of the image decryption capabilities, we are hosting
a test image at,

`docker.io/enccont/encrypted_image:encrypted`

Go ahead and try to download this image using the read-only credentials given below,

```shell
crictl -r unix:///var/run/crio/crio.sock pull docker.io/enccont/encrypted_image:encrypted
```

Since we haven't provided `CRI-O` the access to the private key required to decrypt
this image you should see a failure like this on your console,

<!-- markdownlint-disable MD013 -->
```text
FATA[0010] pulling image failed: rpc error: code = Unknown desc = Error decrypting layer sha256:ecbef970c60906b9d4249b47273113ef008b91ce8046f6ae9d82761b9ffcc3c0: missing private key needed for decryption
```
<!-- markdownlint-enable MD013 -->

This is intended behavior and proof that indeed the encrypted images cannot be
used without having access to the correct private key.

The image can be decrypted using following key,

```text
-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQDoJBuK1hQ5aCbF93uE6jzRm8v5icUNFL5j+DO9hnM5j/8XFTzp
40N2M2/ObLf2qwmWSivwj5LJR/+5ceS8jqVBAcJpckwOXupu3A5o4KgJo15s6v57
4+0wfraNJ/OapqBc7lGFBsj+XwdmegwYYqy41DnYNSzYS4Mov+v7RI014wIDAQAB
AoGALCuiqfouAvZUWlrKv/Gp/OA+IY8bVW/bAj6Z6bgJeKxzhzrdSkuZ7IXBAnAh
WOgWfOhEEBPhhDcU635GXbJusuD/bLBJPOTxiwCFazffm8zVGSQCndfTVxgCM4hn
+5bH6o/cSGQ4E6SLJQeEr8y/J0bMlNMkOco9F1FL1ZgwXGECQQD9/mDwWLJjbdEa
jmGtoPspGz80XDb1jRI09jKDXB826/cBUD+X/P50aTkU+XSJXVfa5F6zhzf/O7C8
07bVnn2pAkEA6fmJ+Jx/Cupy7jRHzIdKAN/7T9QJBIXVDZLz5ulFWLjYkNotpkxk
f0ZSIOvlD7vv5lOifRFivd680XjxIATWqwJARJ35QFUl9DiRuhPnDYok8Cj9PT8A
VfwDhC1S3iv//s1mkINGeuANOhPHKQRvWEDQYEE72FJabWiJyamEhldn6QJAFjLw
3j+q5hQ8d1FKhqNHaDHYHEjX2jAAeNs6fOwhAjv3gDbTIfYZiuHXJPx8rTN9nXLN
9ePSZIVfkNhSuGD9JQJBAI+mobcxj7WkdLHuATdAso+N89Yt7xHoG49c8gz81ufP
vvLPtYytL4ftpiVO3fTfPP90ze8qYPiNaFqMHYDkQ+M=
-----END RSA PRIVATE KEY-----
```

Please save this key in the folder `/etc/crio/keys` with the name of your choice.

Now that we have the private key that can be used by `CRI-O`, let's try to
download the image again,

```shell
crictl -r unix:///var/run/crio/crio.sock pull docker.io/enccont/encrypted_image:encrypted
```

If `CRI-O` was able to read the keys, it would have decrypted the image.
You should see something like this on your console,

```text
Image is up to date for docker.io/enccont/encrypted_image@sha256:2c3c078642b13e34069e55adfd8b93186950860383e49bdeab4858b4a4bdb1bd
```

Verify that image indeed got downloaded and decrypted using
`crictl -r unix:///var/run/crio/crio.sock images`

```text
IMAGE                                   TAG                 IMAGE ID            SIZE
docker.io/enccont/encrypted_image       encrypted           5eb6083c55f01       130MB
```

Please note that the confidentiality provided by the encrypted images could get
compromised if the private keys are accessed by unauthorized and/or unintended entities.
Also, in case of loss of private key, there is no way to access the contents of
the encrypted image rendering it completely unusable. Hence, it's extremely important
to keep the private keys securely and safely.
