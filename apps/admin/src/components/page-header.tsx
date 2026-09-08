'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { createPortal } from 'react-dom';

/**
 * The page's own title, lifted into the top bar.
 *
 * The bar needs to know what page it is sitting above, and only the page knows
 * — a section name from the nav is right for a list and wrong for the detail
 * page underneath it. Rather than have forty pages each reach into the shell,
 * PageTitle publishes here and the shell reads.
 *
 * Title and subtitle travel through state, because they are strings and change
 * rarely. The action buttons do not: they are rebuilt on every render of the
 * page and carry live handlers, so putting them in state would set state on
 * every render and never stop. They are portalled into a slot the shell owns
 * instead — same place on screen, no loop, and the handlers stay attached to
 * the page that made them.
 */
export interface PageHeader {
  title: string;
  subtitle?: string;
}

interface HeaderChannel {
  header: PageHeader | null;
  publish: (header: PageHeader | null) => void;
  /** The DOM node in the top bar that page actions are rendered into. */
  slot: HTMLElement | null;
  setSlot: (node: HTMLElement | null) => void;
}

const Channel = createContext<HeaderChannel>({
  header: null,
  publish: () => {},
  slot: null,
  setSlot: () => {},
});

export function PageHeaderProvider({ children }: { children: ReactNode }) {
  const [header, setHeader] = useState<PageHeader | null>(null);
  const [slot, setSlot] = useState<HTMLElement | null>(null);

  const publish = useCallback((next: PageHeader | null) => {
    setHeader((current) => {
      // Only a real change is a change. Publishing an equal header on every
      // render would re-render the whole shell for nothing.
      if (current?.title === next?.title && current?.subtitle === next?.subtitle) {
        return current;
      }
      return next;
    });
  }, []);

  const value = useMemo(
    () => ({ header, publish, slot, setSlot }),
    [header, publish, slot],
  );
  return <Channel.Provider value={value}>{children}</Channel.Provider>;
}

/** Read the current page's title. For the shell. */
export function usePageHeader(): PageHeader | null {
  return useContext(Channel).header;
}

/** The shell hands its actions container to the pages through this. */
export function usePageActionSlot(): (node: HTMLElement | null) => void {
  return useContext(Channel).setSlot;
}

/**
 * Publish this page's title, and render its actions into the shell's bar.
 *
 * Returns the portal, which PageTitle renders. Until the shell's slot exists
 * — the first paint, or a page rendered outside the shell — the actions
 * simply do not appear, rather than appearing in the wrong place.
 */
export function usePublishHeader(title: string, subtitle: string | undefined, actions: ReactNode) {
  const { publish, slot } = useContext(Channel);

  useEffect(() => {
    publish({ title, subtitle });
    // Clearing on the way out stops a stale title flashing over the next page.
    return () => publish(null);
  }, [publish, title, subtitle]);

  if (!actions || !slot) return null;
  return createPortal(actions, slot);
}
