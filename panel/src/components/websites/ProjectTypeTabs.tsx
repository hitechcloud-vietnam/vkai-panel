'use client';

/**
 * The horizontal strip that chooses a project type.
 *
 * It is a tablist in the accessibility tree, but the selected tab lives in the
 * URL rather than in component state, so a link to the Node.js tab opens on the
 * Node.js tab and the browser's back button steps between tabs. That is the
 * behaviour the Databases screen has; this is a deliberate copy of the
 * convention and imports nothing from it.
 */

import type { ProjectTypeDef, ProjectTypeId } from './projectTypes';

export interface ProjectTypeTabsProps {
  types: ProjectTypeDef[];
  active: ProjectTypeId;
  onSelect: (id: ProjectTypeId) => void;
  /** Row count per type, when it is known. A type still loading passes null. */
  counts: Partial<Record<ProjectTypeId, number | null>>;
}

export default function ProjectTypeTabs({ types, active, onSelect, counts }: ProjectTypeTabsProps) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
      <div
        role="tablist"
        aria-label="Project type"
        className="flex flex-wrap items-center gap-1 overflow-x-auto px-2 py-2"
      >
        {types.map((type) => {
          const selected = type.id === active;
          const count = counts[type.id];
          return (
            <button
              key={type.id}
              type="button"
              role="tab"
              id={`project-tab-${type.id}`}
              aria-selected={selected}
              aria-controls={`project-panel-${type.id}`}
              onClick={() => onSelect(type.id)}
              className={[
                'inline-flex items-center gap-2 whitespace-nowrap rounded-md px-3.5 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500',
                selected
                  ? 'bg-brand-600 text-white'
                  : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
              ].join(' ')}
            >
              <span>{type.label}</span>
              {typeof count === 'number' && (
                <span
                  className={[
                    'rounded px-1.5 py-0.5 text-xs font-semibold',
                    selected ? 'bg-brand-700 text-white' : 'bg-gray-100 text-gray-600',
                  ].join(' ')}
                >
                  {count}
                </span>
              )}
              {type.backend === 'missing' && (
                <span
                  title="No backend manages this project type yet."
                  className={[
                    'rounded px-1.5 py-0.5 text-xs font-medium',
                    selected ? 'bg-brand-700 text-white' : 'bg-gray-100 text-gray-500',
                  ].join(' ')}
                >
                  Not available
                </span>
              )}
              {type.backend === 'partial' && (
                <span
                  title="The panel records this project type but does not yet apply it to the web server."
                  className={[
                    'rounded px-1.5 py-0.5 text-xs font-medium',
                    selected ? 'bg-brand-700 text-white' : 'bg-amber-50 text-amber-700',
                  ].join(' ')}
                >
                  Partial
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
