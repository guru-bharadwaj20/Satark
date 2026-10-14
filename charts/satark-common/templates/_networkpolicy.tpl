{{/*
Default-deny NetworkPolicy for a service, on by default.

Ingress allowed on the http port from:
  - pods listed in networkPolicy.ingressFrom (NetworkPolicyPeer objects)
  - Envoy Gateway pods when networkPolicy.allowGateway is true
  - the monitoring namespace (Prometheus scrapes)
Egress allowed to:
  - kube-dns (UDP/TCP 53)
  - satark-data (PostgreSQL 5432, Valkey 6379, NATS 4222) when
    networkPolicy.allowData is true
  - anything listed in networkPolicy.egress (NetworkPolicyEgressRule objects)
*/}}
{{- define "satark-common.networkpolicy" -}}
{{- $np := .Values.networkPolicy | default dict -}}
{{- if or (not (hasKey $np "enabled")) $np.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      {{- include "satark-common.selectorLabels" . | nindent 6 }}
  policyTypes: ["Ingress", "Egress"]
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
        {{- if $np.allowGateway }}
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: {{ $np.gatewayNamespace | default "envoy-gateway-system" }}
        {{- end }}
        {{- with $np.ingressFrom }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      ports:
        - port: http
          protocol: TCP
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    {{- if $np.allowData }}
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: satark-data
      ports:
        - port: 5432
          protocol: TCP
        - port: 6379
          protocol: TCP
        - port: 4222
          protocol: TCP
    {{- end }}
    {{- with $np.egress }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
{{- end }}
{{- end -}}
