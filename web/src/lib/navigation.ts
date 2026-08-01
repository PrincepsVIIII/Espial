export type SectionNavigationItem = {
  label: string;
  href: string;
};

export function alertNavigationItems(
  permissions: readonly string[],
): SectionNavigationItem[] {
  return [
    { label: 'Active', href: '/alerts' },
    { label: 'History', href: '/alerts/history' },
    ...(permissions.includes('incident_rules:manage')
      ? [{ label: 'Rules', href: '/alerts/rules' }]
      : []),
    ...(permissions.includes('suppressions:manage')
      ? [{ label: 'Suppressions', href: '/alerts/suppressions' }]
      : []),
    ...(permissions.includes('notification_destinations:manage')
      ? [{ label: 'Notifications', href: '/alerts/notifications' }]
      : []),
  ];
}
