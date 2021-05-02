import { Link, Router, useLocation } from '@reach/router';
import React from 'react';
import Home from './pages/Home';

const routes = {
  '/': 'Home',
  http: 'HTTP',
  twitchbot: 'TwitchBot',
  stulbe: 'Stulbe',
  streamlabs: 'StreamLabs',
};

export default function App(): React.ReactElement {
  const loc = useLocation();

  return (
    <div className="container">
      <div className="tabs">
        <ul>
          {Object.entries(routes).map(([route, name]) => (
            <li
              key={route}
              className={loc.pathname === `${route}` ? 'is-active' : ''}
            >
              <Link to={route}>{name}</Link>
            </li>
          ))}
        </ul>
      </div>
      <div className="content">
        <Router>
          <Home path="/" />
        </Router>
      </div>
    </div>
  );
}
