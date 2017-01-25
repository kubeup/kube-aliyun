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

Deploy to Aliyun
----------------

### aliyun-controller

1. Update the required fields in `manifests/aliyun-controller.yaml`
2. Upload it to `/etc/kubernetes/manifests` on all your master nodes
3. Use docker logs to check if the controller is running properly

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


