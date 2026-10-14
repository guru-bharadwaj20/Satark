{{/*
HTTPRoute attaching the service to the public Gateway, when route.enabled.
Values:
  route.gateway.{name,namespace,sectionName}  default satark-public / envoy-gateway-system / https
  route.hostnames                              list of hostnames
  route.paths                                  list of {path, type} (type default PathPrefix)
  route.timeout                                request timeout (default 15s)
*/}}
{{- define "satark-common.httproute" -}}
{{- if and .Values.route .Values.route.enabled }}
{{- $gw := .Values.route.gateway | default dict }}
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
spec:
  parentRefs:
    - name: {{ $gw.name | default "satark-public" }}
      namespace: {{ $gw.namespace | default "envoy-gateway-system" }}
      sectionName: {{ $gw.sectionName | default "https" }}
  {{- with .Values.route.hostnames }}
  hostnames:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  rules:
    - matches:
        {{- range .Values.route.paths | default (list (dict "path" "/")) }}
        - path:
            type: {{ .type | default "PathPrefix" }}
            value: {{ .path }}
        {{- end }}
      backendRefs:
        - name: {{ include "satark-common.fullname" . }}
          port: {{ .Values.service.port }}
      timeouts:
        request: {{ .Values.route.timeout | default "15s" }}
{{- end }}
{{- end -}}
