import { describe, expect, it } from 'vitest';

import { clusterFormFeedback } from './proxmox-form';

describe('Proxmox cluster form feedback', () => {
  it('turns certificate failures into an actionable message', () => {
    expect(
      clusterFormFeedback({
        status: 400,
        code: 'validation_failed',
        message: 'Request validation failed.',
        request_id: 'request-1',
        fields: [{ field: 'proxmox', code: 'tls_certificate_untrusted' }],
      }),
    ).toEqual({
      title: 'Cluster validation failed.',
      details: [
        'The HTTPS certificate is not trusted. If this is the expected self-signed certificate, enable “Trust this cluster’s self-signed certificate.”',
      ],
      requestID: 'request-1',
    });
  });

  it('explains field validation without exposing internal field codes', () => {
    const feedback = clusterFormFeedback({
      status: 400,
      code: 'validation_failed',
      message: 'Request validation failed.',
      fields: [
        { field: 'deployment_id', code: 'invalid' },
        { field: 'storage_thresholds', code: 'invalid_range' },
      ],
    });
    expect(feedback.details).toEqual([
      'Deployment ID must be 2–63 lowercase letters, numbers, or hyphens, beginning with a letter.',
      'Storage warning must be lower than critical, and both must be from 0% to 100%.',
    ]);
  });
});
