'use client';

/**
 * SSH.
 *
 * There is no SSH endpoint in this panel. Not a partial one, not a read-only
 * one: nothing in core/internal reads or writes sshd_config, enumerates
 * authorized_keys, or lists the sessions on the box.
 *
 * So this pane renders no port field, no root-login switch, no password-auth
 * toggle and no key upload. A port box that accepts 2222 and saves nothing is
 * not a smaller version of this feature - it is a worse one, because an
 * operator who believes the port moved will stop watching port 22.
 *
 * What it does render is the requirement that has to be built into the
 * endpoint, not bolted onto the form afterwards: changing the SSH port or
 * turning off password authentication can lock an operator out of the machine
 * for good, and neither may be applied unless the panel has first proved that
 * a working key exists for an account that can still get in.
 */

import { KeyRound, ShieldAlert } from 'lucide-react';

import { BackendGap, Panel, PaneHeading, SectionHeader } from './chrome';

export function SshPane() {
  return (
    <div className="space-y-4">
      <PaneHeading
        title="SSH"
        description="The SSH daemon's port, root login, password authentication, authorised keys and current sessions. None of it is wired: this panel has no endpoint that reads or writes sshd_config."
      />

      <Panel>
        <SectionHeader
          title="Not available"
          description="What this section will hold, and what has to exist behind it first."
        />
        <BackendGap
          title="The panel cannot manage SSH on this host"
          summary="No route under /api/v1 touches sshd_config, authorized_keys, or the SSH session list, and no service in core/internal implements them. Nothing on this screen can report the current SSH configuration, so nothing on this screen pretends to."
          controls={[
            'Listening port, with the firewall rule for the new port added before the daemon is moved',
            'PermitRootLogin: yes, no, or prohibit-password',
            'PasswordAuthentication on or off',
            'Authorised keys per account: list, add, remove, with the fingerprint and the comment',
            'Current sessions: user, source address, terminal, login time, with the ability to end one',
            'Recent accepted and failed SSH logins read from the host auth log',
          ]}
          endpoints={[
            'GET  /api/v1/ssh/config',
            'PUT  /api/v1/ssh/config',
            'GET  /api/v1/ssh/keys',
            'POST /api/v1/ssh/keys',
            'DELETE /api/v1/ssh/keys/:id',
            'POST /api/v1/ssh/keys/verify',
            'GET  /api/v1/ssh/sessions',
            'DELETE /api/v1/ssh/sessions/:id',
          ]}
          note="Until these exist, SSH on this machine is managed over SSH itself. The Overview marks SSH password authentication and root login as not verifiable rather than guessing at them."
        />
      </Panel>

      <Panel>
        <SectionHeader
          title="The rule the endpoint has to enforce"
          description="Written here so it is designed into the API rather than added to the form later."
        />
        <div className="space-y-4 px-4 py-4">
          <div className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-4">
            <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden="true" />
            <div className="min-w-0 text-sm text-red-800">
              <p className="font-semibold text-red-900">
                Two of these settings can lock you out of the machine permanently.
              </p>
              <p className="mt-1">
                Moving the SSH port without opening the new one in the firewall, and turning off
                password authentication on an account that has no key, both end the same way: the
                panel keeps working until it is restarted, and nobody can reach a shell on the box
                again. On a rented server that is a support ticket to the data centre.
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3 rounded-md border border-gray-200 bg-gray-50 p-4">
            <KeyRound className="mt-0.5 h-5 w-5 shrink-0 text-gray-500" aria-hidden="true" />
            <div className="min-w-0 text-sm text-gray-700">
              <p className="font-semibold text-gray-900">
                So the endpoint must refuse the change until it has proved the way back in.
              </p>
              <ul className="mt-2 list-disc space-y-1 pl-5">
                <li>
                  Disabling password authentication is refused unless at least one account that is
                  permitted to log in has a key in authorized_keys that the server has parsed and
                  accepted &mdash; a file that exists is not proof; a key the daemon rejects is not
                  proof.
                </li>
                <li>
                  Changing the port is refused unless a firewall rule already admits the new port,
                  and the daemon is reloaded rather than restarted so an open session survives to
                  undo it.
                </li>
                <li>
                  Both changes keep the previous configuration and roll back automatically if the
                  operator does not confirm from a NEW connection within a few minutes. Confirming
                  from the session that made the change proves nothing about the next one.
                </li>
                <li>
                  The refusal is stated plainly in the response, not returned as a generic 400: the
                  operator has to know it was refused because the panel could not find them a key.
                </li>
              </ul>
            </div>
          </div>
        </div>
      </Panel>
    </div>
  );
}

export default SshPane;
