{{/*
Restricted security context by default (Pod Security Standard "restricted").
Charts may override individual fields through podSecurityContext and
securityContext; anything not overridden keeps the restricted default.
A writable emptyDir is mounted at /tmp because the root filesystem is
read-only (disable with tmpVolume: false).
*/}}
{{- define "satark-common.podSecurityContext" -}}
{{- $defaults := dict
  "runAsNonRoot" true
  "runAsUser" 65532
  "runAsGroup" 65532
  "fsGroup" 65532
  "seccompProfile" (dict "type" "RuntimeDefault")
-}}
{{- toYaml (mergeOverwrite $defaults (.Values.podSecurityContext | default dict)) -}}
{{- end -}}

{{- define "satark-common.containerSecurityContext" -}}
{{- $defaults := dict
  "allowPrivilegeEscalation" false
  "readOnlyRootFilesystem" true
  "runAsNonRoot" true
  "privileged" false
  "capabilities" (dict "drop" (list "ALL"))
  "seccompProfile" (dict "type" "RuntimeDefault")
-}}
{{- toYaml (mergeOverwrite $defaults (.Values.securityContext | default dict)) -}}
{{- end -}}

{{- define "satark-common.tmpVolumeEnabled" -}}
{{- if or (not (hasKey .Values "tmpVolume")) .Values.tmpVolume -}}true{{- end -}}
{{- end -}}
