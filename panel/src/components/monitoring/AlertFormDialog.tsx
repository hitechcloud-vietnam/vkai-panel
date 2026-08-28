'use client';

/**
 * Creating a monitoring alert rule.
 *
 * Every field here matches models.CreateMonitoringAlertRequest, and the
 * constraints are the server's, not inventions:
 *
 *  - name, metric, condition, threshold and severity are bound with
 *    binding:"required". Because Go's `required` rejects a zero value, a
 *    threshold of exactly 0 is refused by the API, which is why the form asks
 *    for a non-zero number rather than sending one that will bounce.
 *  - condition is one of gt, gte, lt, lte, eq, ne. Anything else is stored and
 *    then never matches, because service.checkAlerts switches on exactly those
 *    six strings.
 *  - server_id is optional in the request and necessary in practice:
 *    checkAlerts looks alerts up by server, so a rule saved without one is
 *    never evaluated. The form therefore defaults to the selected node and says
 *    why it matters.
 */

import { useEffect, useState } from 'react';
import { X } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import ServerScopeField from '@/components/servers/ServerScopeField';
import { monitoringApi } from '@/services/api';
import { errorMessage } from '@/lib/apiError';
import type { ManagedServer } from '@/types/server';

/** The six comparisons service.checkAlerts understands. */
const CONDITIONS: { value: string; label: string }[] = [
  { value: 'gt', label: 'is greater than' },
  { value: 'gte', label: 'is greater than or equal to' },
  { value: 'lt', label: 'is less than' },
  { value: 'lte', label: 'is less than or equal to' },
  { value: 'eq', label: 'equals' },
  { value: 'ne', label: 'does not equal' },
];

const SEVERITIES = ['info', 'warning', 'critical'];

const SELECT_CLASS =
  'h-9 w-full rounded-md border border-gray-300 bg-white px-3 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';

export interface AlertFormDialogProps {
  open: boolean;
  servers: ManagedServer[];
  /** The node the page is showing; the rule starts pointed at it. */
  defaultServerId: string;
  /** Metric names already in use, offered as suggestions. */
  metricSuggestions: string[];
  onClose: () => void;
  onCreated: () => void;
}

export default function AlertFormDialog({
  open,
  servers,
  defaultServerId,
  metricSuggestions,
  onClose,
  onCreated,
}: AlertFormDialogProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [metric, setMetric] = useState('');
  const [condition, setCondition] = useState('gt');
  const [threshold, setThreshold] = useState('');
  const [duration, setDuration] = useState('300');
  const [severity, setSeverity] = useState('warning');
  const [serverId, setServerId] = useState(defaultServerId);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName('');
    setDescription('');
    setMetric('');
    setCondition('gt');
    setThreshold('');
    setDuration('300');
    setSeverity('warning');
    setServerId(defaultServerId);
    setError(null);
    setBusy(false);
  }, [open, defaultServerId]);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const submit = async () => {
    const thresholdValue = Number(threshold);
    if (!name.trim()) {
      setError('Give the rule a name.');
      return;
    }
    if (!metric.trim()) {
      setError('Name the metric this rule watches, for example cpu_usage.');
      return;
    }
    if (!Number.isFinite(thresholdValue) || thresholdValue === 0) {
      setError('Enter a threshold. The API rejects a threshold of 0, because the field is bound as required.');
      return;
    }

    setBusy(true);
    setError(null);
    try {
      await monitoringApi.createAlert({
        name: name.trim(),
        description: description.trim(),
        metric: metric.trim(),
        condition,
        threshold: thresholdValue,
        duration: Number(duration) > 0 ? Number(duration) : 300,
        severity,
        server_id: serverId || '',
      });
      onCreated();
      onClose();
    } catch (err) {
      setError(errorMessage(err, 'The alert rule could not be created.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-gray-900/40 p-4 sm:p-8">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="New alert rule"
        className="w-full max-w-xl rounded-lg border border-gray-200 bg-white shadow-sm"
      >
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-5 py-4">
          <div>
            <h2 className="text-sm font-semibold text-gray-900">New alert rule</h2>
            <p className="mt-1 text-sm text-gray-600">
              A rule is checked when a sample is recorded for its metric on its node.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="shrink-0 rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            <X size={16} aria-hidden="true" />
          </button>
        </div>

        <div className="space-y-4 px-5 py-4">
          <div>
            <label htmlFor="alert-name" className="mb-1.5 block text-sm font-medium text-gray-700">
              Name
            </label>
            <Input
              id="alert-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="CPU above 90 per cent"
            />
          </div>

          <ServerScopeField
            id="alert-server"
            servers={servers}
            value={serverId}
            onChange={setServerId}
          />
          <p className="-mt-2 text-xs text-gray-500">
            A rule without a node is stored but never evaluated: the checker looks rules up by
            server.
          </p>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="alert-metric" className="mb-1.5 block text-sm font-medium text-gray-700">
                Metric
              </label>
              <Input
                id="alert-metric"
                value={metric}
                onChange={(event) => setMetric(event.target.value)}
                list="alert-metric-suggestions"
                placeholder="cpu_usage"
                spellCheck={false}
                autoComplete="off"
              />
              <datalist id="alert-metric-suggestions">
                {metricSuggestions.map((suggestion) => (
                  <option key={suggestion} value={suggestion} />
                ))}
              </datalist>
              <p className="mt-1 text-xs text-gray-500">
                Matched exactly against the name a sample is recorded under.
              </p>
            </div>

            <div>
              <label htmlFor="alert-severity" className="mb-1.5 block text-sm font-medium text-gray-700">
                Severity
              </label>
              <select
                id="alert-severity"
                value={severity}
                onChange={(event) => setSeverity(event.target.value)}
                className={SELECT_CLASS}
              >
                {SEVERITIES.map((value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div className="sm:col-span-2">
              <label htmlFor="alert-condition" className="mb-1.5 block text-sm font-medium text-gray-700">
                Fires when the value
              </label>
              <select
                id="alert-condition"
                value={condition}
                onChange={(event) => setCondition(event.target.value)}
                className={SELECT_CLASS}
              >
                {CONDITIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="alert-threshold" className="mb-1.5 block text-sm font-medium text-gray-700">
                Threshold
              </label>
              <Input
                id="alert-threshold"
                value={threshold}
                onChange={(event) => setThreshold(event.target.value)}
                inputMode="decimal"
                placeholder="90"
              />
            </div>
          </div>

          <div>
            <label htmlFor="alert-duration" className="mb-1.5 block text-sm font-medium text-gray-700">
              Duration in seconds
            </label>
            <Input
              id="alert-duration"
              value={duration}
              onChange={(event) => setDuration(event.target.value)}
              inputMode="numeric"
            />
            <p className="mt-1 text-xs text-gray-500">
              Stored with the rule; the API defaults it to 300 when left at zero.
            </p>
          </div>

          <div>
            <label htmlFor="alert-description" className="mb-1.5 block text-sm font-medium text-gray-700">
              Description
            </label>
            <textarea
              id="alert-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              rows={2}
              className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            />
          </div>

          {error ? (
            <p className="text-sm text-red-700" role="alert">
              {error}
            </p>
          ) : null}
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2 border-t border-gray-200 px-5 py-3">
          <Button type="button" variant="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="button" onClick={submit} disabled={busy}>
            {busy ? 'Creating...' : 'Create rule'}
          </Button>
        </div>
      </div>
    </div>
  );
}
