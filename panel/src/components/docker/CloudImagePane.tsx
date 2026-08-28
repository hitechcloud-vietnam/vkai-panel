'use client';

/**
 * Cloud image: pull an image by name and tag, and watch the layers arrive.
 *
 * The route is mounted. `POST /api/v1/docker/images/pull` validates that a
 * repository was supplied, defaults the tag to `latest`, writes a log line and
 * answers `{"message": "Image pull started"}`. Nothing is pulled.
 *
 * That is exactly the failure this panel keeps repeating, and it is worse here
 * than elsewhere: the response says "started", so a form wired to it would look
 * correct - a spinner, then a success toast - and the image would simply never
 * appear on the Local image tab. An operator would conclude the panel is slow,
 * not that it is lying. So no form is rendered until the handler pulls.
 *
 * Progress needs more than the handler: a pull reports per-layer status over
 * time, which needs either a job the panel can poll or a WebSocket. Neither
 * exists, so both are in the backlog below.
 */

import { PaneUnavailable } from './CapabilityNotice';
import type { CapabilityGap } from '@/types/docker';

const GAPS: CapabilityGap[] = [
  {
    label: 'Pull an image',
    missing:
      'POST /api/v1/docker/images/pull is mounted, but PullImage in core/internal/handler/docker.go logs the repository and tag and answers "Image pull started" without contacting a registry.',
  },
  {
    label: 'Pull progress',
    missing:
      'A pull is long-running and reports per-layer status. There is no job record to poll and no WebSocket channel for it, so a progress bar has no source. The panel already has a WebSocket handler (core/internal/handler/websocket.go) that this could reuse.',
  },
  {
    label: 'Search a registry',
    missing:
      'No route searches Docker Hub or any other registry, so an image name has to be typed exactly.',
  },
  {
    label: 'Pull from a configured registry',
    missing:
      'The pull request takes only a repository and tag. It has no registry field, so an image on a private registry could not be authenticated even once registries are stored (see the Repository tab).',
  },
];

const WOULD_SHOW = [
  'A repository and tag box with the resolved reference shown before it is pulled',
  'Which configured registry the pull will authenticate against',
  'Per-layer progress while it downloads, and the total transferred',
  'The finished image handed straight to the Local image tab',
];

export function CloudImagePane() {
  return (
    <PaneUnavailable
      title="Pulling an image is not implemented"
      summary={
        <>
          The pull route exists and answers{' '}
          <span className="font-mono text-xs">
            &#123;&quot;message&quot;: &quot;Image pull started&quot;&#125;
          </span>{' '}
          for any repository you send it — while pulling nothing. A form wired to that
          would show a spinner and then success, and the image would never appear under
          Local image. That is why there is no form here yet.
        </>
      }
      wouldShow={WOULD_SHOW}
      gaps={GAPS}
    />
  );
}

export default CloudImagePane;
