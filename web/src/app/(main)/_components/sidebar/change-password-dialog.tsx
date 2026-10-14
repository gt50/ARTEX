"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

export function ChangePasswordDialog({
  open,
  onOpenChange,
}: {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
}) {
  const [oldPassword, setOldPassword] = React.useState("");
  const [username, setUsername] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const [loadError, setLoadError] = React.useState("");
  const [newPassword, setNewPassword] = React.useState("");
  const [confirmPassword, setConfirmPassword] = React.useState("");
  const [saving, setSaving] = React.useState(false);

  // reset fields whenever the dialog closes
  React.useEffect(() => {
    if (!open) {
      setOldPassword("");
      setNewPassword("");
      setConfirmPassword("");
    }
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    let active = true;
    setLoading(true);
    setLoadError("");
    api.account().then((account) => {
      if (active) setUsername(account.username);
    }).catch((err) => {
      if (active) setLoadError((err as Error).message);
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [open]);

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (loading || loadError) return;
    if (!oldPassword || !username.trim()) {
      toast.error("Enter your username and current password");
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error("The new passwords do not match");
      return;
    }
    setSaving(true);
    api
      .changePassword(oldPassword, newPassword, username.trim())
      .then(({ token }) => {
        auth.setToken(token);
        toast.success("Account updated");
        onOpenChange(false);
        window.location.reload();
      })
      .catch((err) => toast.error(`Failed to update account: ${(err as Error).message}`))
      .finally(() => setSaving(false));
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>Account settings</DialogTitle>
            <DialogDescription>
              Change your username or password. Enter your current password to save changes.
              Leave the new password blank to keep your password. Existing sessions remain signed in.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3 py-4">
            {loadError && <p role="alert" className="text-sm text-destructive">Unable to load account: {loadError}</p>}
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cp-username">Username</Label>
              <Input id="cp-username" name="username" autoComplete="username" value={username} disabled={saving || loading} onChange={(e) => setUsername(e.target.value)} required maxLength={64} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cp-old">Current password</Label>
              <Input
                id="cp-old"
                type="password"
                autoComplete="current-password"
                value={oldPassword}
                disabled={saving}
                onChange={(e) => setOldPassword(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cp-new">New password</Label>
              <Input
                id="cp-new"
                type="password"
                autoComplete="new-password"
                value={newPassword}
                disabled={saving}
                onChange={(e) => setNewPassword(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cp-confirm">Confirm new password</Label>
              <Input
                id="cp-confirm"
                type="password"
                autoComplete="new-password"
                value={confirmPassword}
                disabled={saving}
                onChange={(e) => setConfirmPassword(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              Cancel
            </Button>
            <Button type="submit" disabled={saving || loading || !!loadError || !username.trim() || !oldPassword}>
              {saving ? "Saving..." : "Save changes"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
