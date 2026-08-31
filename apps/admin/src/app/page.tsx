'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAdminAuth } from '../lib/auth';

export default function IndexPage() {
  const router = useRouter();
  const token = useAdminAuth((s) => s.token);
  useEffect(() => {
    router.replace(token ? '/dashboard' : '/login');
  }, [router, token]);
  return null;
}
