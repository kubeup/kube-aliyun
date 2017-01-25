Kube Aliyun
===========

[![CircleCI](https://circleci.com/gh/kubeup/kube-aliyun/tree/master.svg?style=shield)](https://circleci.com/gh/kubeup/kube-aliyun)

Aliyun essentials for Kubernetes. It provides SLB, Routes controllers and a Volume
plugin for Kubernetes to function properly on Aliyun instances.

Features
--------

* Service load balancers sync (TCP & UDP)
* Routes sync
* Volumes attaching & mounting (via flexvolume for now)

Components
----------

There are two components. They run independently.

**aliyun-controller** is a daemon responsible for service & route synchronization.
It has to run on all master nodes.

**aliyun-flexv** is a binary plugin responsible for attaching & mounting volumes
 on node. It has to be deployed on all nodes and will be called by kubelets when
 needed.

Docker Image
------------

kubeup/kube-aliyun


Deploy to Aliyun
----------------

### aliyun-controller

1. Make sure all node names are internal ip addresses.
2. Make sure node cidr will be allocated by adding `--allocate-node-cidrs=true 
--configure-cloud-routes=false` to `kube-controller-manager` commandline.
3. Update the required fields in `manifests/aliyun-controller.yaml`
4. Upload it to `/etc/kubernetes/manifests` on all your master nodes
5. Use docker logs to check if the controller is running properly

### aliyun-flexv

1. Update the kubelet systemd file on every node to include these required environment variables:

  - ALIYUN_ACCESS_KEY
  - ALIYUN_ACCESS_KEY_SECRET
  - ALIYUN_REGION
  - ALIYUN_INSTANCE (the instance id of the node in Aliyun)
  
2. Add to kubelet commandline an option `--volume-plugin-dir=/opt/k8s/volume/plugins`
3. Run on each node

```bash
  FLEXPATH=/opt/k8s/volume/plugins/aliyun~flexv; sudo mkdir $FLEXPATH -p; docker run -v $FLEXPATH:/opt kubeup/kube-aliyun:master cp /flexv /opt/
```

* Customizing volume plugin path is optional. You can just use the default which is
`/usr/libexec/kubernetes/kubelet-plugins/volume/exec/`.

Usage
-----

### Services 

Just create Loadbalancer Services as usual. Currently only TCP & UDP types are 
supported. Some options can be customized through annotaion on Service. Please 
see `pkg/cloudprovider/providers/aliyun/loadbalancer.go` for details.

### Routes

No flannel or other network layer needed. Pods should be routed just fine.

### Flex Volume

In pod definition, use `flexVolume` type as in the example and that's it.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx
    volumeMounts:
    - name: test
      mountPath: /data
    ports:
    - containerPort: 80
  volumes:
  - name: test
    flexVolume:
      driver: "aliyun/flexv"
      fsType: "ext4"
      options:
        diskId: "d-1ierokwer8234jowe"
```

License
-------

Apache Version 2.0
