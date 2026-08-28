'use client';

/**
 * Creating a database on an engine the service can actually drive.
 *
 * Every rule the form enforces is one the Go side enforces too, quoted in
 * passwordRules.ts and in types/databases.ts. Charset and collation are offered
 * as pickers rather than as free text because validateDBRequest checks them
 * against two fixed allowlists - a typed value outside them is a 500, and a
 * dropdown of the eight values that work is kinder than an error message.
 *
 * The charset and collation fields belong to MySQL. The PostgreSQL pane hides
 * them: createPostgresDatabase ignores both, and offering an operator a control
 * whose value the server discards is the same lie as a button wired to nothing.
 */

import { useEffect, useMemo, useState } from 'react';
import { Loader2, RefreshCw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';
import type { ManagedServer } from '@/types/server';
import {
  ALLOWED_CHARSETS,
  ALLOWED_COLLATIONS,
  type CreateDatabasePayload,
  type DBServer,
  type DatabaseEngine,
} from '@/types/databases';

import Modal from './Modal';
import { PANE_INPUT_CLASS, PRIMARY_BUTTON_CLASS, SECONDARY_BUTTON_CLASS } from './PaneChrome';
import { generatePassword, identifierProblem, passwordProblem } from './passwordRules';
import { instancePort, nodeName } from './helpers';

const SELECT_TRIGGER_CLASS =
  'border-gray-300 bg-white text-gray-900 focus:ring-1 focus:ring-brand-500 focus:ring-offset-0';

export interface CreateDatabaseDialogProps {
  open: boolean;
  engine: DatabaseEngine;
  /** Instances of this engine that a database can be created on. */
  servers: DBServer[];
  nodes: ManagedServer[];
  onClose: () => void;
  /**
   * Performs the create. The password is handed back so the caller can hold it
   * for this browser session - the API will never return it again.
   */
  onCreate: (payload: CreateDatabasePayload) => Promise<void>;
}

export function CreateDatabaseDialog({
  open,
  engine,
  servers,
  nodes,
  onClose,
  onCreate,
}: CreateDatabaseDialogProps) {
  const usesCharset = engine.id === 'mysql';

  const [serverId, setServerId] = useState('');
  const [name, setName] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [charset, setCharset] = useState<string>(ALLOWED_CHARSETS[0]);
  const [collation, setCollation] = useState<string>(ALLOWED_COLLATIONS[0]);
  const [showPassword, setShowPassword] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (!open) return;
    setServerId(servers.length === 1 ? String(servers[0]?.id || '') : '');
    setName('');
    setUsername('');
    setPassword(generatePassword());
    setCharset(ALLOWED_CHARSETS[0]);
    setCollation(ALLOWED_COLLATIONS[0]);
    setShowPassword(false);
    setError(null);
    setTouched(false);
  }, [open, servers]);

  const nameProblem = useMemo(
    () => (touched || name ? identifierProblem(name, 'database name') : null),
    [name, touched]
  );
  const userProblem = useMemo(
    () => (touched || username ? identifierProblem(username, 'user name') : null),
    [username, touched]
  );
  const passProblem = useMemo(
    () => (touched || password ? passwordProblem(password) : null),
    [password, touched]
  );

  const canSubmit =
    Boolean(serverId) &&
    !identifierProblem(name, 'database name') &&
    !identifierProblem(username, 'user name') &&
    !passwordProblem(password) &&
    !saving;

  const submit = async () => {
    setTouched(true);
    if (!canSubmit) return;
    setSaving(true);
    setError(null);
    try {
      await onCreate({
        database_server_id: serverId,
        name: name.trim(),
        username: username.trim(),
        password,
        ...(usesCharset ? { charset, collation } : {}),
      });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      title={`Create a ${engine.label} database`}
      description={`The panel creates the database, the ${engine.accountNoun.toLowerCase()}, and the grant between them in one step.`}
      onClose={saving ? () => undefined : onClose}
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={saving}
            className={SECONDARY_BUTTON_CLASS}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={submit}
            disabled={saving}
            className={PRIMARY_BUTTON_CLASS}
          >
            {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            Create database
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div>
          <label
            htmlFor="db-create-server"
            className="mb-1.5 block text-sm font-medium text-gray-700"
          >
            Instance
          </label>
          <Select value={serverId} onValueChange={setServerId}>
            <SelectTrigger id="db-create-server" className={SELECT_TRIGGER_CLASS}>
              <SelectValue placeholder="Choose the instance to create it on" />
            </SelectTrigger>
            <SelectContent className="border-gray-200 bg-white">
              {servers.map((server) => (
                <SelectItem key={server.id} value={server.id}>
                  {nodeName(server, nodes)} · {server.type} :{instancePort(server, engine)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {servers.length === 0 && (
            <p className="mt-1 text-xs text-amber-700">
              No {engine.label} instance is registered yet, so there is nowhere to create
              this database.
            </p>
          )}
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label
              htmlFor="db-create-name"
              className="mb-1.5 block text-sm font-medium text-gray-700"
            >
              Database name
            </label>
            <Input
              id="db-create-name"
              value={name}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => setName(e.target.value)}
              placeholder="app_production"
              className={cn(PANE_INPUT_CLASS, 'font-mono')}
            />
            {nameProblem && <p className="mt-1 text-xs text-red-600">{nameProblem}</p>}
          </div>

          <div>
            <label
              htmlFor="db-create-user"
              className="mb-1.5 block text-sm font-medium text-gray-700"
            >
              {engine.accountNoun} name
            </label>
            <Input
              id="db-create-user"
              value={username}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="app_user"
              className={cn(PANE_INPUT_CLASS, 'font-mono')}
            />
            {userProblem && <p className="mt-1 text-xs text-red-600">{userProblem}</p>}
          </div>
        </div>

        <div>
          <label
            htmlFor="db-create-password"
            className="mb-1.5 block text-sm font-medium text-gray-700"
          >
            Password
          </label>
          <div className="flex gap-2">
            <Input
              id="db-create-password"
              type={showPassword ? 'text' : 'password'}
              value={password}
              autoComplete="new-password"
              onChange={(e) => setPassword(e.target.value)}
              className={cn(PANE_INPUT_CLASS, 'font-mono')}
            />
            <Button
              type="button"
              variant="outline"
              onClick={() => setShowPassword((v) => !v)}
              className={SECONDARY_BUTTON_CLASS}
            >
              {showPassword ? 'Hide' : 'Show'}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setPassword(generatePassword())}
              aria-label="Generate a new password"
              className={SECONDARY_BUTTON_CLASS}
            >
              <RefreshCw className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
          {passProblem ? (
            <p className="mt-1 text-xs text-red-600">{passProblem}</p>
          ) : (
            <p className="mt-1 text-xs text-gray-500">
              Copy it now. The panel stores it encrypted and the API never returns it
              again.
            </p>
          )}
        </div>

        {usesCharset && (
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label
                htmlFor="db-create-charset"
                className="mb-1.5 block text-sm font-medium text-gray-700"
              >
                Character set
              </label>
              <Select value={charset} onValueChange={setCharset}>
                <SelectTrigger id="db-create-charset" className={SELECT_TRIGGER_CLASS}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="border-gray-200 bg-white">
                  {ALLOWED_CHARSETS.map((value) => (
                    <SelectItem key={value} value={value}>
                      {value}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <label
                htmlFor="db-create-collation"
                className="mb-1.5 block text-sm font-medium text-gray-700"
              >
                Collation
              </label>
              <Select value={collation} onValueChange={setCollation}>
                <SelectTrigger id="db-create-collation" className={SELECT_TRIGGER_CLASS}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="border-gray-200 bg-white">
                  {ALLOWED_COLLATIONS.map((value) => (
                    <SelectItem key={value} value={value}>
                      {value}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="mt-1 text-xs text-gray-500">
                Only these eight collations are accepted by the server.
              </p>
            </div>
          </div>
        )}

        {error && (
          <p className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
            {error}
          </p>
        )}
      </div>
    </Modal>
  );
}

export default CreateDatabaseDialog;
