import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import sdk from './api';
import type { ClusterStatus } from './api';

interface ClusterContextValue {
  status: ClusterStatus | null;
  error: string | null;
  isLoading: boolean;
  refresh: () => Promise<void>;
}

const ClusterContext = createContext<ClusterContextValue>({
  status: null,
  error: null,
  isLoading: true,
  refresh: async () => {},
});

export const useCluster = () => useContext(ClusterContext);

export const ClusterProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const refresh = useCallback(async () => {
    setIsLoading(true);
    try {
      const data = await sdk.getStatus();
      setStatus(data);
      setError(null);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  return (
    <ClusterContext.Provider value={{ status, error, isLoading, refresh }}>
      {children}
    </ClusterContext.Provider>
  );
};
