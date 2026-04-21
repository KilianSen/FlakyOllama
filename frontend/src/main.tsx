import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router';
import './index.css';
import { router } from './router';
import { ClusterProvider } from './ClusterContext';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ClusterProvider>
      <RouterProvider router={router} />
    </ClusterProvider>
  </StrictMode>,
);
