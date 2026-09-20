import React from "react";
import Header from "./header";

export default function Info() {
  return (
    <div className="info">
      <Header />
      <div className="info-text">
        <p>
          Trimetric is a realtime visualization of Delhi&apos;s bus network,
          built on the open data published by{" "}
          <a href="https://otd.delhi.gov.in">Open Transit Data Delhi</a>. It
          covers DTC and DIMTS cluster buses. The view you are currently
          looking at shows the location of every vehicle and bus stop. The
          orange dots are vehicles, and the black dots are stops.{" "}
        </p>
        <p>
          If you zoom in, you can see more information about the stops,
          including upcoming arrivals.{" "}
        </p>
        <p>
          The data for the view comes from the static and realtime{" "}
          <a href="https://gtfs.org">GTFS</a> feeds published on the Open
          Transit Data portal by the Government of NCT of Delhi.
        </p>
        <p className="info-text-credits">
          Trimetric was originally built for Portland&apos;s TriMet network by{" "}
          <a href="https://briand.co">Brian Davidson</a>. It&apos;s open source
          and you can find the original at
          <a href="https://github.com/bsdavidson/trimetric">
            {" "}
            github.com/bsdavidson/trimetric
          </a>
        </p>
      </div>
      <div className="info-icons">
        <div className="info-icons-text">
          Trimetric is powered by these technologies:
        </div>
        <a href="https://reactjs.org/" title="React">
          <img
            alt="React"
            className="info-icon react"
            src="/assets/react.svg"
          />
        </a>
        <a href="https://redux.js.org/" title="Redux">
          <img
            alt="Redux"
            className="info-icon redux"
            src="/assets/redux.svg"
          />
        </a>
        <a href="https://golang.org/" title="Go">
          <img alt="Go" className="info-icon go" src="/assets/go.svg" />
        </a>
        <a href="https://www.docker.com/" title="Docker">
          <img
            alt="Docker"
            className="info-icon docker"
            src="/assets/docker.svg"
          />
        </a>
        <a href="https://www.postgresql.org/" title="Postgres">
          <img
            alt="Postgres"
            className="info-icon postgresql"
            src="/assets/postgresql.svg"
          />
        </a>
      </div>
    </div>
  );
}
