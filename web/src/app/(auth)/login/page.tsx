"use client";

import { useEffect, useRef, useState } from "react";

import { useRouter } from "next/navigation";

import { AlertTriangle, ShieldCheck } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [agreed, setAgreed] = useState(false);
  const [termsOpen, setTermsOpen] = useState(false);
  const [readToEnd, setReadToEnd] = useState(false);
  const termsBodyRef = useRef<HTMLDivElement>(null);

  // Require the user to reach the end of the terms (including when they fit without scrolling).
  function handleTermsScroll() {
    const el = termsBodyRef.current;
    if (!el) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 8) setReadToEnd(true);
  }

  useEffect(() => {
    if (!termsOpen) return;
    // Reset on open and handle terms that fit on screen without a scroll event.
    setReadToEnd(false);
    const el = termsBodyRef.current;
    if (el && el.scrollHeight <= el.clientHeight + 8) setReadToEnd(true);
  }, [termsOpen]);

  useEffect(() => {
    // Redirect authenticated users to the app (static exports have no middleware for this).
    const token = auth.getToken();
    if (token) {
      // localStorage may still contain a token after the cookie was lost. Restore it before
      // navigating so server guards or route caching do not send the user back to this page.
      auth.setToken(token);
      window.location.replace("/function/tasks");
      return;
    }
    api
      .authStatus()
      .then(({ initialized }) => {
        if (!initialized) router.replace("/setup");
      })
      .catch(() => setError("Unable to connect to the backend"))
      .finally(() => setChecking(false));
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!agreed) {
      setError("Please read and accept the Terms of Use first");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const { token } = await api.login(username.trim(), password);
      auth.setToken(token);
      window.location.replace("/function/tasks");
    } catch {
      setError("Incorrect username or password");
    } finally {
      setLoading(false);
    }
  }

  if (checking) {
    return (
      <div role="status" className="flex min-h-dvh items-center justify-center text-muted-foreground">
        Checking sign-in status...
      </div>
    );
  }

  return (
    <div className="flex h-dvh">
      {/* Left panel */}
      <div className="hidden flex-col items-center justify-center bg-primary p-12 text-center lg:flex lg:w-1/3">
        <div className="relative flex items-center justify-center">
          <div className="absolute size-80 rounded-full border border-primary-foreground/10" />
          <div className="absolute size-60 rounded-full border border-primary-foreground/15" />
          <div className="absolute size-40 rounded-full border border-primary-foreground/20" />
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/logo.png" alt="ARTEX" width={160} height={160} className="relative brightness-0 invert" />
        </div>
      </div>

      {/* Right panel */}
      <div className="flex w-full items-center justify-center bg-background p-8 lg:w-2/3">
        <div className="w-full max-w-md space-y-10 py-24 lg:py-32">
          <div className="space-y-4 text-center">
            <h2 className="text-2xl font-medium tracking-tight">Sign In</h2>
            <p className="mx-auto max-w-xl text-muted-foreground">Welcome back. Enter your username and password to continue.</p>
          </div>
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="username">Username</Label>
              <Input id="username" name="username" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="Enter your username" autoComplete="username" autoFocus required maxLength={64} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                name="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password"
                required
                autoComplete="current-password"
              />
            </div>
            <div className="flex items-start gap-2">
              <Checkbox
                id="agree-terms"
                checked={agreed}
                onCheckedChange={(v) => setAgreed(v === true)}
                className="mt-0.5"
              />
              <Label htmlFor="agree-terms" className="text-sm font-normal leading-relaxed text-muted-foreground">
                I have read and agree to the
                <button
                  type="button"
                  onClick={() => setTermsOpen(true)}
                  className="mx-0.5 font-medium text-primary underline-offset-4 hover:underline"
                >
                  Terms of Use
                </button>
              </Label>
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading || !username.trim() || !password || !agreed}>
              {loading ? "Signing in..." : "Sign In"}
            </Button>
          </form>
        </div>
      </div>

      <Dialog open={termsOpen} onOpenChange={setTermsOpen}>
        <DialogContent className="gap-0 p-0 sm:max-w-2xl">
          <DialogHeader className="flex-row items-center gap-3 border-b px-6 py-4">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <ShieldCheck className="size-5" />
            </div>
            <div className="space-y-0.5">
              <DialogTitle className="text-base">ARTEX Terms of Use and Disclaimer</DialogTitle>
              <p className="text-xs text-muted-foreground">
                Version 1.0 · Effective date: 2026-09-18 · Read all terms before signing in
              </p>
            </div>
          </DialogHeader>

          <div
            ref={termsBodyRef}
            onScroll={handleTermsScroll}
            className="max-h-[60vh] space-y-5 overflow-y-auto px-6 py-5 text-sm leading-relaxed text-muted-foreground"
          >
            <p className="rounded-lg border bg-muted/40 p-3 text-foreground/80">
              These Terms of Use and Disclaimer (the "Terms") form an agreement between you and the
              authors and contributors of ARTEX regarding your use of this software. Read and understand
              every provision before using the software, especially the disclaimers, limitations of
              liability, and prohibited uses highlighted below.
              <span className="font-medium text-foreground">
                {" "}
                By downloading, installing, accessing, or otherwise using this software, you confirm
                that you have read, understood, and agreed to be bound by these Terms.
              </span>
            </p>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  1
                </span>
                1. Definitions and Open-Source License
              </h4>
              <p className="pl-7">
                ARTEX is open-source software distributed under the GNU Affero General Public License
                v3.0 (AGPL-3.0). You may use, copy, modify, and distribute it under that license.
                Derivative works, including services made available to third parties over a network,
                must also be licensed under AGPL-3.0 and provide users with the complete corresponding
                source code. The accompanying LICENSE file contains the full license terms.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  2
                </span>
                2. Permitted Use
              </h4>
              <p className="pl-7">
                This software may be used only for personal learning, source-code research, discussion
                of security principles, and technical validation in an isolated local environment that
                you set up. Permitted purposes include education, academic research, and code review.
                You may not use the software for any other purpose.
              </p>
            </section>

            <section className="space-y-2">
              <h4 className="flex items-center gap-2 font-medium text-destructive">
                <span className="flex size-5 items-center justify-center rounded-md bg-destructive/10 text-xs font-semibold text-destructive">
                  3
                </span>
                <AlertTriangle className="size-4" />
                3. Prohibited Conduct
              </h4>
              <ul className="ml-7 list-decimal space-y-1.5 rounded-lg border border-destructive/20 bg-destructive/5 p-3 pl-8 text-foreground/80 marker:text-destructive/70">
                <li>
                  Do not scan, probe, exploit, or attack any website, online service, or connected
                  system, regardless of whether you own it or have authorization;
                </li>
                <li>Do not use this software for real-world penetration testing, offensive or defensive operations, red-team or blue-team exercises, or in production environments;</li>
                <li>Do not use this software for unlawful access, data theft, extortion, denial-of-service (DoS/DDoS), or any destructive or criminal activity;</li>
                <li>Do not remove, alter, or circumvent copyright, license, or safety notices in this software or its output;</li>
                <li>Do not violate laws, regulations, or regulatory requirements in your jurisdiction.</li>
              </ul>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  4
                </span>
                4. Intellectual Property
              </h4>
              <p className="pl-7">
                Copyright and other intellectual property rights in this software belong to its authors
                and contributors. You receive the rights granted under AGPL-3.0. No other rights are
                granted expressly or by implication under these Terms.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  5
                </span>
                5. Data and Privacy
              </h4>
              <p className="pl-7">
                This is self-hosted open-source software. The authors do not operate a centralized
                service and do not collect or upload usage data. You control all data created,
                processed, or accessed through your use of the software and are responsible for its
                lawful and secure handling and for any consequences of mishandling it.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  6
                </span>
                6. Compliance and Legal Responsibility
              </h4>
              <p className="pl-7">
                You are responsible for complying with all laws and regulations in your jurisdiction
                concerning cybersecurity, data protection, personal information, and computer crime.
                <span className="font-medium text-foreground">
                  {" "}
                  You alone are responsible for legal liability and consequences arising from your
                  violation of applicable law or these Terms. The authors and contributors are not
                  responsible for such conduct.
                </span>
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  7
                </span>
                7. Disclaimer and Limitation of Liability
              </h4>
              <p className="pl-7">
                This software is provided "AS IS" and "AS AVAILABLE," without warranties of any kind,
                express or implied, including warranties of merchantability, fitness for a particular
                purpose, accuracy, or non-infringement. To the maximum extent permitted by applicable
                law, the authors and contributors are not liable for direct, indirect, incidental,
                special, or consequential damages arising from using or being unable to use this
                software, whether used properly or improperly. This includes data loss, system damage,
                business interruption, lost profits, and legal disputes.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  8
                </span>
                8. Changes to These Terms
              </h4>
              <p className="pl-7">
                The authors may update these Terms to reflect changes in law or the project. Updated
                Terms will be published with the project and take effect on publication. Continued use
                of the software means you accept the revised Terms. To the extent permitted by law,
                the authors retain final authority to interpret these Terms. If a provision is found
                invalid, the remaining provisions continue in effect.
              </p>
            </section>
          </div>

          <DialogFooter className="mx-0 mb-0 flex-col items-stretch gap-2 rounded-b-xl px-6 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs text-muted-foreground">
              {readToEnd ? "You have reached the end of the terms" : "Scroll to the end of the terms to continue"}
            </p>
            <DialogClose asChild>
              <Button
                type="button"
                disabled={!readToEnd}
                onClick={() => {
                  setAgreed(true);
                  setError("");
                }}
              >
                I have read and agree to all terms
              </Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
