# AWS account setup

One-time, manual procedure. Terraform cannot do it, because this step creates
the identities Terraform runs as later. Finish it before

## 1. Root user

1. Sign up for a new AWS account using a dedicated mailbox alias, not a
   personal address.
2. Turn on MFA for the root user: **IAM → Security credentials → Assign MFA
   device**. Use a FIDO2 security key or a TOTP app, and keep the recovery
   codes offline.
3. Make sure the root user has **no access keys**. Delete any that exist.
4. Set the alternate contacts for billing, operations and security.
5. Turn on **IAM user and role access to Billing information** under Account
   settings, so the admin identity can see the budget created in

After this step, only use the root user for tasks that AWS reserves for root.

## 2. Separate admin identity

Preferred: **IAM Identity Center** (SSO).

1. Turn on IAM Identity Center in the home region (`ap-south-1`).
2. Create a user `satark-admin` and require MFA (Settings → Authentication →
   "Every time they sign in", using a security key or an authenticator app).
3. Create an `AdministratorAccess` permission set with a 1h session duration,
   and assign it to `satark-admin` on this account.
4. Configure the CLI:

   ```sh
   aws configure sso --profile satark-admin
   aws sso login --profile satark-admin
   aws sts get-caller-identity --profile satark-admin
   ```

Fallback if Identity Center is not available: an IAM user `satark-admin` in a
group with `AdministratorAccess`, with console MFA enforced. Create access keys
only if you really need them, and rotate them every 90 days.

## 3. Account hygiene

- Turn on the account-level S3 Block Public Access setting.
- Turn on default EBS encryption in `ap-south-1`.
- Turn on Cost Explorer and the Free Tier usage alerts.
- Keep CloudTrail's default 90-day event history. An organisation trail is out
  of scope for this project.

## 4. Verification checklist

- [ ] Root MFA is on and root has no access keys
- [ ] `aws sts get-caller-identity --profile satark-admin` returns the admin identity
- [ ] The admin identity can see billing
- [ ] The account-level S3 public access block is on
- [ ] Default EBS encryption is on

Next: apply `infra/terraform/modules/budget-alarm` before any other
resource.
