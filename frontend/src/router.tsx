import { createBrowserRouter } from 'react-router';
import App from './App';
import { OverviewPage } from './pages/OverviewPage';
import { FleetPage } from './pages/FleetPage';
import { RegistryPage } from './pages/RegistryPage';
import { KeysPage } from './pages/KeysPage';
import { PublicPortal } from './pages/PublicPortal';
import { PlaygroundPage } from './pages/PlaygroundPage';
import { ChatPage } from './pages/ChatPage';
import { LogsPage } from './pages/LogsPage';
import { ConfigPage } from './pages/ConfigPage';
import ProfilePage from './pages/ProfilePage';
import { UsersPage } from './pages/UsersPage';

export const router = createBrowserRouter([
  {
    path: '/',
    element: <App />,
    children: [
      { index: true, element: <OverviewPage /> },
      { path: 'fleet', element: <FleetPage /> },
      { path: 'registry', element: <RegistryPage /> },
      { path: 'users', element: <UsersPage /> },
      { path: 'keys', element: <KeysPage /> },
      { path: 'portal', element: <PublicPortal /> },
      { path: 'playground', element: <PlaygroundPage /> },
      { path: 'chat', element: <ChatPage /> },
      { path: 'logs', element: <LogsPage /> },
      { path: 'profile', element: <ProfilePage /> },
      { path: 'config', element: <ConfigPage /> },
    ],
  },
]);
