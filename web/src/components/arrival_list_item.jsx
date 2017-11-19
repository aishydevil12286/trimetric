import React from "react";
import {withRouter} from "react-router-dom";
import {connect} from "react-redux";

import {TrimetricPropTypes} from "./prop_types";
import {degreeToCompass} from "../helpers/directions";
import {updateLocation, LocationTypes} from "../actions";

export class ArrivalListItem extends React.Component {
  constructor(props) {
    super(props);
    this.handleVehicleClick = this.handleVehicleClick.bind(this);
  }

  componentWillUnmount() {
    if (
      this.props.locationClicked &&
      this.props.arrival.vehicle_id === this.props.locationClicked.id
    ) {
      this.props.clearLocation(this.props.location);
    }
  }

  handleVehicleClick() {
    if (window) {
      window.scrollTo(0, 0);
    }
    if (!this.props.arrival.vehicle_id) {
      return;
    }
    this.props.onVehicleClick(LocationTypes.VEHICLE, this.props.arrival);
  }

  render() {
    let routeClass = "";
    if (
      this.props.locationClicked &&
      this.props.locationClicked.following &&
      this.props.locationClicked.id === this.props.arrival.vehicle_id
    ) {
      routeClass = "active";
    }

    let routeStyle = {
      backgroundColor: this.props.color
    };

    let travelInfo = null;
    if (this.props.arrival.vehicle_id) {
      travelInfo = (
        <div className="arrival-metric arrival-direction">
          Traveling:{" "}
          {degreeToCompass(this.props.arrival.vehicle_position.bearing)}
        </div>
      );
    } else {
      travelInfo = <div className="arrival-metric arrival-direction">n/a</div>;
    }

    return (
      <div
        className={"arrival-list-item " + routeClass}
        onClick={this.handleVehicleClick}>
        <div className="arrival-id" style={routeStyle}>
          {this.props.arrival.route_id}
        </div>
        <div className="arrival-name-metrics">
          <div className="arrival-name">
            {this.props.arrival.vehicle_label || this.props.arrival.headsign}
          </div>
          <div className="arrival-metrics">
            <div className="arrival-metric arrival-est-time">
              {this.props.arrivalTime}
            </div>

            {travelInfo}
          </div>
        </div>
      </div>
    );
  }
}

ArrivalListItem.propTypes = {
  arrival: TrimetricPropTypes.arrival,
  arrivalTime: TrimetricPropTypes.arrivalTime,
  location: TrimetricPropTypes.location,
  locationClicked: TrimetricPropTypes.locationClicked
};

function mapDispatchToProps(dispatch) {
  return {
    onVehicleClick: (type, arrival) => {
      dispatch(
        updateLocation(
          type,
          arrival.vehicle_id,
          arrival.vehicle_position.lat,
          arrival.vehicle_position.lng,
          true
        )
      );
    },
    clearLocation: location => {
      dispatch(
        updateLocation(
          LocationTypes.HOME,
          null,
          location.lat,
          location.lng,
          false
        )
      );
    }
  };
}

function mapStateToProps(state) {
  return {
    location: state.location,
    locationClicked: state.locationClicked,
    vehicles: state.vehicles
  };
}

export default withRouter(
  connect(mapStateToProps, mapDispatchToProps)(ArrivalListItem)
);
