'use client';

/**
 * Template, Subscribers and Groups - the three list screens behind a campaign.
 *
 * Each is backed by real CRUD, but each is missing its update route, so a row
 * can be created and deleted and not corrected. That is called out where it
 * bites rather than hidden behind an Edit button that would 404.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight, Plus, Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { errorMessage } from '@/lib/apiError';

import { marketingApi } from './api';
import {
  ActionError,
  BackendGap,
  Dash,
  EmptyState,
  ErrorBlock,
  Field,
  Modal,
  Notice,
  Panel,
  Pill,
  ROW,
  SectionHeader,
  TableScroller,
  TableSkeleton,
  TD,
  TH,
  Toolbar,
} from './chrome';
import { contactStatusTone, formatDate, isSuspended } from './format';
import { useResource } from './useResource';
import type { EmailContact, EmailList, EmailTemplate } from './types';

/* ------------------------------------------------------------------ *
 * Templates
 * ------------------------------------------------------------------ */

export function TemplatesPane() {
  const load = useCallback(() => marketingApi.listTemplates(), []);
  const state = useResource<EmailTemplate[]>(load, 'Could not load templates.');

  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<EmailTemplate | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [form, setForm] = useState({ name: '', subject: '', category: '', html_content: '' });

  const templates = useMemo(() => state.data ?? [], [state.data]);
  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return templates;
    return templates.filter(
      (t) =>
        t.name.toLowerCase().includes(term) ||
        t.subject.toLowerCase().includes(term) ||
        (t.category ?? '').toLowerCase().includes(term)
    );
  }, [templates, search]);

  const openCreate = () => {
    setForm({ name: '', subject: '', category: '', html_content: '' });
    setActionError('');
    setOpen(true);
  };

  const submit = async () => {
    if (!form.name.trim() || !form.subject.trim() || !form.html_content.trim()) {
      setActionError('Name, subject and body are all required by the backend.');
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      await marketingApi.createTemplate({
        name: form.name.trim(),
        subject: form.subject.trim(),
        category: form.category.trim() || undefined,
        html_content: form.html_content,
      });
      setOpen(false);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not save the template.'));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (template: EmailTemplate) => {
    if (!window.confirm(`Delete the template "${template.name}"?`)) return;
    setActionError('');
    try {
      await marketingApi.deleteTemplate(template.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not delete the template.'));
    }
  };

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Templates"
          description="Reusable message bodies. A campaign does not read one automatically yet; copy the body across."
        />

        <Notice tone="amber">
          There is no update route for templates, so a saved template can only be replaced by
          deleting it and creating another. The missing route is{' '}
          <span className="font-mono text-xs">PUT /api/v1/email-marketing/templates/:id</span>.
        </Notice>

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search templates"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <Button type="button" onClick={openCreate}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Template
          </Button>
        </Toolbar>

        {actionError && !open && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={4} rows={4} />}

        {!state.loading && state.error && (
          <ErrorBlock title="Could not load templates" message={state.error} onRetry={state.reload} />
        )}

        {!state.loading && !state.error && rows.length === 0 && (
          <EmptyState
            title={search ? 'No template matches that search' : 'No templates yet'}
            description={
              search
                ? 'Clear the search to see every template.'
                : 'Save a message body you send often, so the next campaign starts from something rather than an empty box.'
            }
            action={
              !search && (
                <Button type="button" onClick={openCreate}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  Add Template
                </Button>
              )
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[720px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Template
                  </th>
                  <th className={TH} scope="col">
                    Subject
                  </th>
                  <th className={TH} scope="col">
                    Category
                  </th>
                  <th className={TH} scope="col">
                    Created
                  </th>
                  <th className={TH} scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((template) => {
                  const created = formatDate(template.created_at);
                  return (
                    <tr key={template.id} className={ROW}>
                      <td className={TD}>
                        <span className="font-medium text-gray-900">{template.name}</span>
                        {template.is_default && (
                          <span className="ml-2">
                            <Pill tone="sky">Default</Pill>
                          </span>
                        )}
                      </td>
                      <td className={TD}>
                        <span className="block max-w-xs truncate" title={template.subject}>
                          {template.subject}
                        </span>
                      </td>
                      <td className={TD}>
                        {template.category || <span className="text-gray-500">None</span>}
                      </td>
                      <td className={TD}>
                        {created ?? <Dash reason="The backend did not report a creation date." />}
                      </td>
                      <td className={TD}>
                        <div className="flex items-center gap-2">
                          <Button
                            type="button"
                            variant="secondary"
                            size="sm"
                            onClick={() => setPreview(template)}
                          >
                            View source
                          </Button>
                          <Button
                            type="button"
                            variant="danger-outline"
                            size="sm"
                            onClick={() => remove(template)}
                            aria-label={`Delete ${template.name}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                            Delete
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Modal
        open={open}
        wide
        title="New template"
        onClose={() => setOpen(false)}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={submit} disabled={busy}>
              {busy ? 'Saving…' : 'Save template'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Name" htmlFor="tpl-name">
            <Input
              id="tpl-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="Monthly newsletter"
            />
          </Field>
          <Field label="Category" htmlFor="tpl-category" hint="Optional. Used only for grouping.">
            <Input
              id="tpl-category"
              value={form.category}
              onChange={(e) => setForm({ ...form, category: e.target.value })}
              placeholder="newsletter"
            />
          </Field>
        </div>
        <Field label="Subject" htmlFor="tpl-subject">
          <Input
            id="tpl-subject"
            value={form.subject}
            onChange={(e) => setForm({ ...form, subject: e.target.value })}
          />
        </Field>
        <Field
          label="Body (HTML)"
          htmlFor="tpl-body"
          hint="Include an unsubscribe link in anything sent in bulk."
        >
          <textarea
            id="tpl-body"
            rows={12}
            className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-xs text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            value={form.html_content}
            onChange={(e) => setForm({ ...form, html_content: e.target.value })}
          />
        </Field>
      </Modal>

      <Modal
        open={preview !== null}
        wide
        title={preview ? preview.name : 'Template'}
        description="Shown as source. The panel does not render stored HTML, so a template cannot run script inside this page."
        onClose={() => setPreview(null)}
        footer={
          <Button type="button" variant="secondary" onClick={() => setPreview(null)}>
            Close
          </Button>
        }
      >
        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs text-gray-700">
          {preview?.html_content}
        </pre>
      </Modal>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Subscribers
 * ------------------------------------------------------------------ */

const PAGE_SIZE = 25;

export function SubscribersPane() {
  const [page, setPage] = useState(0);
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');

  // The list endpoint searches server-side, so the typing is debounced rather
  // than firing a request per keystroke.
  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(0);
    }, 350);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const load = useCallback(
    () => marketingApi.listContacts(PAGE_SIZE, page * PAGE_SIZE, search),
    [page, search]
  );
  const state = useResource(load, 'Could not load subscribers.', [page, search]);

  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [form, setForm] = useState({ email: '', first_name: '', last_name: '', tags: '' });

  const contacts = state.data?.items ?? [];
  const total = state.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const submit = async () => {
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(form.email.trim())) {
      setActionError('Enter a valid email address. The backend rejects anything else.');
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      await marketingApi.createContact({
        email: form.email.trim().toLowerCase(),
        first_name: form.first_name.trim(),
        last_name: form.last_name.trim(),
        tags: form.tags
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
        source: 'manual',
      });
      setOpen(false);
      setForm({ email: '', first_name: '', last_name: '', tags: '' });
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not add the subscriber.'));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (contact: EmailContact) => {
    const extra = isSuspended(contact.status)
      ? '\n\nThis address is on the suspend list. Deleting it removes the record that it must not be mailed, so a later import could add it back and mail it again.'
      : '';
    if (!window.confirm(`Delete ${contact.email}?${extra}`)) return;
    setActionError('');
    try {
      await marketingApi.deleteContact(contact.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not delete the subscriber.'));
    }
  };

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Subscribers"
          description="Every address this account holds, whatever its state."
        />

        <Notice tone="amber">
          A subscriber&apos;s details and status cannot be changed once saved: the module has no
          update route (<span className="font-mono text-xs">PUT /api/v1/email-marketing/contacts/:id</span>
          ), so unsubscribing or re-subscribing somebody from the panel is not possible yet.
        </Notice>

        <Toolbar
          search={searchInput}
          onSearchChange={setSearchInput}
          searchPlaceholder="Search by address or name"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <Button type="button" onClick={() => { setActionError(''); setOpen(true); }}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Subscriber
          </Button>
        </Toolbar>

        {actionError && !open && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={5} rows={6} />}

        {!state.loading && state.error && (
          <ErrorBlock
            title="Could not load subscribers"
            message={state.error}
            onRetry={state.reload}
          />
        )}

        {!state.loading && !state.error && contacts.length === 0 && (
          <EmptyState
            title={search ? 'No subscriber matches that search' : 'No subscribers yet'}
            description={
              search
                ? 'Clear the search to see every subscriber.'
                : 'Add the people a campaign will go to. Only add addresses that asked to hear from you: a purchased list is the fastest route to a blocklisted server.'
            }
            action={
              !search && (
                <Button type="button" onClick={() => setOpen(true)}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  Add Subscriber
                </Button>
              )
            }
          />
        )}

        {!state.loading && !state.error && contacts.length > 0 && (
          <>
            <TableScroller>
              <table className="w-full min-w-[820px] border-collapse">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className={TH} scope="col">
                      Address
                    </th>
                    <th className={TH} scope="col">
                      Name
                    </th>
                    <th className={TH} scope="col">
                      Status
                    </th>
                    <th className={TH} scope="col">
                      Source
                    </th>
                    <th className={TH} scope="col">
                      Added
                    </th>
                    <th className={TH} scope="col">
                      <span className="sr-only">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {contacts.map((contact) => {
                    const added = formatDate(contact.created_at);
                    const name = [contact.first_name, contact.last_name]
                      .filter(Boolean)
                      .join(' ')
                      .trim();
                    return (
                      <tr key={contact.id} className={ROW}>
                        <td className={TD}>
                          <span className="font-medium text-gray-900">{contact.email}</span>
                          {(contact.tags ?? []).length > 0 && (
                            <div className="mt-1 flex flex-wrap gap-1">
                              {(contact.tags ?? []).map((tag) => (
                                <Pill key={tag} tone="gray">
                                  {tag}
                                </Pill>
                              ))}
                            </div>
                          )}
                        </td>
                        <td className={TD}>
                          {name || <span className="text-gray-500">Not given</span>}
                        </td>
                        <td className={TD}>
                          <Pill tone={contactStatusTone(contact.status)}>
                            {contact.status || 'unknown'}
                          </Pill>
                        </td>
                        <td className={TD}>{contact.source || 'manual'}</td>
                        <td className={TD}>
                          {added ?? <Dash reason="The backend did not report a creation date." />}
                        </td>
                        <td className={TD}>
                          <Button
                            type="button"
                            variant="danger-outline"
                            size="sm"
                            onClick={() => remove(contact)}
                            aria-label={`Delete ${contact.email}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                            Delete
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </TableScroller>

            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 px-4 py-3">
              <p className="text-sm text-gray-500">
                {total} subscriber{total === 1 ? '' : 's'} · page {page + 1} of {pages}
              </p>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                  disabled={page === 0}
                >
                  <ChevronLeft className="h-3.5 w-3.5" aria-hidden="true" />
                  Previous
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={() => setPage((p) => p + 1)}
                  disabled={page + 1 >= pages}
                >
                  Next
                  <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
                </Button>
              </div>
            </div>
          </>
        )}
      </Panel>

      <Modal
        open={open}
        title="Add subscriber"
        onClose={() => setOpen(false)}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={submit} disabled={busy}>
              {busy ? 'Adding…' : 'Add subscriber'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <Field label="Email address" htmlFor="sub-email">
          <Input
            id="sub-email"
            type="email"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
            placeholder="person@example.vn"
            autoComplete="off"
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="First name" htmlFor="sub-first">
            <Input
              id="sub-first"
              value={form.first_name}
              onChange={(e) => setForm({ ...form, first_name: e.target.value })}
            />
          </Field>
          <Field label="Last name" htmlFor="sub-last">
            <Input
              id="sub-last"
              value={form.last_name}
              onChange={(e) => setForm({ ...form, last_name: e.target.value })}
            />
          </Field>
        </div>
        <Field label="Tags" htmlFor="sub-tags" hint="Comma separated. Optional.">
          <Input
            id="sub-tags"
            value={form.tags}
            onChange={(e) => setForm({ ...form, tags: e.target.value })}
            placeholder="customer, hanoi"
          />
        </Field>
      </Modal>
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Groups
 * ------------------------------------------------------------------ */

export function GroupsPane() {
  const load = useCallback(() => marketingApi.listGroups(), []);
  const state = useResource<EmailList[]>(load, 'Could not load groups.');

  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const [form, setForm] = useState({ name: '', description: '', double_opt_in: true });

  const groups = useMemo(() => state.data ?? [], [state.data]);
  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return groups;
    return groups.filter(
      (g) => g.name.toLowerCase().includes(term) || (g.description ?? '').toLowerCase().includes(term)
    );
  }, [groups, search]);

  const submit = async () => {
    if (!form.name.trim()) {
      setActionError('Give the group a name.');
      return;
    }
    setBusy(true);
    setActionError('');
    try {
      await marketingApi.createGroup({
        name: form.name.trim(),
        description: form.description.trim(),
        double_opt_in: form.double_opt_in,
      });
      setOpen(false);
      setForm({ name: '', description: '', double_opt_in: true });
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not create the group.'));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (group: EmailList) => {
    if (!window.confirm(`Delete the group "${group.name}"? The subscribers themselves are kept.`)) {
      return;
    }
    setActionError('');
    try {
      await marketingApi.deleteGroup(group.id);
      state.reload();
    } catch (err) {
      setActionError(errorMessage(err, 'Could not delete the group.'));
    }
  };

  return (
    <div className="space-y-4">
      <Panel>
        <SectionHeader
          title="Groups"
          description="Named audiences a campaign can be aimed at."
        />

        <Notice tone="amber">
          A group can be created and deleted, but nobody can be put in one: the join table
          (email_list_contacts) has a migration and no routes, so the member count below stays at
          zero. Missing:{' '}
          <span className="font-mono text-xs">POST /api/v1/email-marketing/lists/:id/contacts</span>{' '}
          and{' '}
          <span className="font-mono text-xs">
            DELETE /api/v1/email-marketing/lists/:id/contacts/:contactId
          </span>
          .
        </Notice>

        <Toolbar
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search groups"
          onRefresh={state.reload}
          refreshing={state.refreshing}
        >
          <Button type="button" onClick={() => { setActionError(''); setOpen(true); }}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            Add Group
          </Button>
        </Toolbar>

        {actionError && !open && (
          <div className="px-4 pt-3">
            <ActionError message={actionError} onDismiss={() => setActionError('')} />
          </div>
        )}

        {state.loading && <TableSkeleton columns={4} rows={3} />}

        {!state.loading && state.error && (
          <ErrorBlock title="Could not load groups" message={state.error} onRetry={state.reload} />
        )}

        {!state.loading && !state.error && rows.length === 0 && (
          <EmptyState
            title={search ? 'No group matches that search' : 'No groups yet'}
            description={
              search
                ? 'Clear the search to see every group.'
                : 'Create a group for each audience you send to separately, so an unsubscribe from one does not silence the rest.'
            }
            action={
              !search && (
                <Button type="button" onClick={() => setOpen(true)}>
                  <Plus className="h-4 w-4" aria-hidden="true" />
                  Add Group
                </Button>
              )
            }
          />
        )}

        {!state.loading && !state.error && rows.length > 0 && (
          <TableScroller>
            <table className="w-full min-w-[720px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <th className={TH} scope="col">
                    Group
                  </th>
                  <th className={TH} scope="col">
                    Members
                  </th>
                  <th className={TH} scope="col">
                    Confirmed opt-in
                  </th>
                  <th className={TH} scope="col">
                    Created
                  </th>
                  <th className={TH} scope="col">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((group) => {
                  const created = formatDate(group.created_at);
                  return (
                    <tr key={group.id} className={ROW}>
                      <td className={TD}>
                        <p className="font-medium text-gray-900">{group.name}</p>
                        {group.description && (
                          <p className="mt-0.5 max-w-md text-xs text-gray-500">
                            {group.description}
                          </p>
                        )}
                      </td>
                      <td className={TD}>{group.contact_count ?? 0}</td>
                      <td className={TD}>
                        {group.double_opt_in ? (
                          <Pill tone="emerald">Required</Pill>
                        ) : (
                          <Pill tone="amber">Not required</Pill>
                        )}
                      </td>
                      <td className={TD}>
                        {created ?? <Dash reason="The backend did not report a creation date." />}
                      </td>
                      <td className={TD}>
                        <Button
                          type="button"
                          variant="danger-outline"
                          size="sm"
                          onClick={() => remove(group)}
                          aria-label={`Delete ${group.name}`}
                        >
                          <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                          Delete
                        </Button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableScroller>
        )}
      </Panel>

      <Panel>
        <SectionHeader title="Group membership" description="Who is in each group." />
        <BackendGap
          title="Subscribers cannot be put into a group yet"
          description="The join table exists in migration 015 but no handler touches it, so there is no way to add or remove a member and no way to list one group's audience. Campaigns therefore have no audience to resolve, which is part of why nothing sends."
          missing={[
            'GET    /api/v1/email-marketing/lists/:id/contacts',
            'POST   /api/v1/email-marketing/lists/:id/contacts',
            'DELETE /api/v1/email-marketing/lists/:id/contacts/:contactId',
          ]}
        />
      </Panel>

      <Modal
        open={open}
        title="New group"
        onClose={() => setOpen(false)}
        footer={
          <>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={submit} disabled={busy}>
              {busy ? 'Creating…' : 'Create group'}
            </Button>
          </>
        }
      >
        <ActionError message={actionError} onDismiss={() => setActionError('')} />
        <Field label="Name" htmlFor="grp-name">
          <Input
            id="grp-name"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            placeholder="Newsletter subscribers"
          />
        </Field>
        <Field label="Description" htmlFor="grp-description" hint="Optional.">
          <Input
            id="grp-description"
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
        </Field>
        <label className="flex items-start gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
            checked={form.double_opt_in}
            onChange={(e) => setForm({ ...form, double_opt_in: e.target.checked })}
          />
          <span>
            Require confirmed opt-in
            <span className="block text-xs text-gray-500">
              Members confirm by clicking a link before they receive anything. It costs sign-ups and
              it is the single best protection against complaints.
            </span>
          </span>
        </label>
      </Modal>
    </div>
  );
}
