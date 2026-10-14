environment           = "dev"
domain                = "dev.satark.example.in"
edge_vpc_cidr         = "10.61.0.0/16"
edge_instance_type    = "t4g.nano"
force_destroy_buckets = true
velero_retention_days = 7
alarm_emails          = ["ops@satark.example.in"]
