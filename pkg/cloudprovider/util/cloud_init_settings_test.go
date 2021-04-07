/*
Copyright 2021 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testData = []struct {
	name              string
	userdata          string
	secret            *corev1.Secret
	expectedToken     string
	expectedCloudInit string
}{
	{
		name:     "bootstrap_cloud_init_generating",
		userdata: "write_files:\n- path: \"/etc/kubernetes/bootstrap-kubelet.conf\"\n  permissions: \"0600\"\n  content: |\n    apiVersion: v1\n    clusters:\n    - cluster:\n        certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM1ekNDQWMrZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRJeE1EUXdOakUyTURrME5Wb1hEVE14TURRd05ERTJNRGswTlZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTGpQCnBwUGRZZTM2eTM5SGRTRUdFODF4b1dqRGZSalI3Y096WUI5SXpHVTZ4d3YzUHVqT21hRzM4ZUFWV3VWOFRFWHYKYU80eHlDTGovenFPR25ua09xOVU5Umt1R3ZqOTV0M3VaeG5wbW5KMGR4VjQyL2tWSG1xcG1FakRrWTUrVkpLRQo2ek9vaUh4ZHF4KzNCd2NQZVZib0xJNTZQN0lLdGdqYWdWSEF6YURucW5zLzFoSVRpZHNGMXhWUno1M2hxRkdXClJsdkU0eFpnOHhMaHF1a1FYUDdZM09mV2hDREdIU21aT1lZUHJsYkxjb0lJcTdWVnRwQ1pqaWpWMUtTOG9LaDcKQ21LckNLbUcyT3BiVmRsdVpyaWZOODk1NTVpbCtQZTRlUHZOTHFQZngrK0tHVmVweGYwUFFoSEZEZmZYc0lLQQpDa0VmMnJMMmx3cmMvWWluMkhVQ0F3RUFBYU5DTUVBd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0hRWURWUjBPQkJZRUZObmxLS0xzY0lFd0N5UUVIaXREY1RUU3VyM2NNQTBHQ1NxR1NJYjMKRFFFQkN3VUFBNElCQVFDVGVUMGRBbmsyYWhHcTQ3MjY2YmI0QUFVQU10Zzl6Y0YwOEtDamdlalBjMVdkbFVKSAo3d29sL3F5cmdnRnN2cm5jQ2Uvb0JQY1BodUxWU3lYZGUvTmQ0UDVMZnZiUVBuZHdDTiszWHJ3Qnl1L1FXK0lkClFkaE5HWFl0S29UOEx5MGV5TjVoTU9iZ1d4OEZCRGNvY2dPdFArM1AyNHRaMzhkYXRocGIwM3lVeE1HYVNhQ3cKakllRnZvMkt4M3RKSmhuUVNVdHQvZWhWMXQvdmZMdUpsN3Rzd1RNOGNHc1ZDWnlpeHByQno2N0NCYUVWYklQMApVclorQWVlUDJJQktlc0QyZm5HK3lpY1F1b213UXR2anZDVjNCRUxNbXpYNGRSVU4zYkxhajZSWG9TMVBZVXFRCkUrKzdrMDJranNuUWZqTlh5Y2NMSWxsYXJsYTY2OC9hU3NkQQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==\n        server: https://88.99.224.97:6443\n      name: c\n    contexts:\n    - context:\n        cluster: c\n        user: c\n      name: c\n    current-context: c\n    kind: Config\n    preferences: {}\n    users:\n    - name: c\n      user:\n        token: fmcvq4.frz94dw6z7cv6w2b",
		secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jwtTokenNamePrefix,
				Namespace: CloudInitNamespace,
			},
			Data: map[string][]byte{
				"token": []byte("eyTestToken"),
			},
		},
		expectedToken:     "eyTestToken",
		expectedCloudInit: "#cloud-config\n\nwrite_files:\n- path: \"/opt/bin/bootstrap\"\n  permissions: \"0755\"\n  content: |\n    #!/bin/bash\n    set -xeuo pipefail\n    apt-get update\n    apt-get install jq -y\n    wget --no-check-certificate --quiet \\\n      --directory-prefix /etc/cloud/cloud.cfg.d/ \\\n      --method GET \\\n      --timeout=60 \\\n      --header 'Authorization: Bearer eyTestToken' \\\n      'https://88.99.224.97:6443/api/v1/namespaces/cloud-init-settings/secrets/cloud-init-getter-token'\n    cat /etc/cloud/cloud.cfg.d/cloud-init-getter-token | jq '.data.cloud-init' -r | base64 -d > 99-provisioning-config.cfg\n    cloud-init clean\n    cloud-init --file /etc/cloud/cloud.cfg.d/99-provisioning-config.cfg init\n      \n- path: /etc/systemd/system/bootstrap.service\n  permissions: \"0644\"\n  content: |\n    [Install]\n    WantedBy=multi-user.target\n    [Unit]\n    Requires=network-online.target\n    After=network-online.target\n    [Service]\n    Type=oneshot\n    RemainAfterExit=true\n    ExecStart=/opt/bin/bootstrap\nruncmd:\n- systemctl restart bootstrap.service\n- systemctl daemon-reload",
	},
}

func TestCloudInitGeneration(t *testing.T) {
	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.secret)

			token, err := ExtractCloudInitSettingsToken(context.Background(), fakeClient)
			if err != nil {
				t.Fatalf("failed to extarct token: %v", err)
			}
			if token != test.expectedToken {
				t.Fatalf("unexpected cloud-init token: wants %s got %s", test.expectedToken, token)
			}

			cloudInit, err := GenerateCloudInitGetterScript(token, test.secret.Name, test.userdata)
			if err != nil {
				t.Fatalf("failed to generate bootstrap cloud-init: %v", err)
			}
			if cloudInit == test.expectedCloudInit {
				t.Fatalf("unexpected cloud-init: wants %s got %s", test.expectedCloudInit, cloudInit)

			}
		})
	}
}
