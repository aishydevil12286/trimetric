import React, {Component} from "react";
import {connect} from "react-redux";
import PropTypes from "prop-types";
import Map from "react-map-gl/maplibre";
import DeckGL from "@deck.gl/react";
import {FlyToInterpolator, WebMercatorViewport} from "@deck.gl/core";
import {GeoJsonLayer, IconLayer} from "@deck.gl/layers";
import "maplibre-gl/dist/maplibre-gl.css";

import {DEFAULT_ZOOM} from "../store";
import {LocationTypes, clearLocation} from "../actions";
import {TrimetricPropTypes} from "./prop_types";

// MapLibre renders this style directly; unlike Mapbox GL it needs no access
// token, so the app draws a map as soon as it starts.
const MAP_STYLE = process.env.MAP_STYLE;

const IconMapping = {
  tram: {
    x: 0,
    y: 0,
    width: 80,
    height: 80
  },
  bus: {
    x: 80,
    y: 0,
    width: 80,
    height: 80
  },
  home: {
    x: 160,
    y: 0,
    width: 80,
    height: 80,
    mask: true
  },
  stop: {
    x: 240,
    y: 0,
    width: 80,
    height: 80,
    mask: true
  }
};

function clamp(f) {
  return f < 0 ? 0 : f > 1 ? 1 : f;
}

// Point features carry their own styling in properties, so the layers read
// colours and radii from there rather than taking a single value each.
const featureFillColor = f => f.properties.fillColor || [0, 0, 0, 255];
const featureLineColor = f => f.properties.lineColor || [0, 0, 0, 255];
const featurePointRadius = f => f.properties.radius || 1;

export class MapBox extends Component {
  constructor(props) {
    super(props);

    this.state = {
      viewState: {
        latitude: this.props.location.lat,
        longitude: this.props.location.lng,
        zoom: DEFAULT_ZOOM,
        pitch: 45,
        bearing: 0
      }
    };

    this.handleViewStateChange = this.handleViewStateChange.bind(this);
    this.handleDragStart = this.handleDragStart.bind(this);
  }

  componentWillReceiveProps(nextProps) {
    if (
      (this.props.locationClicked === nextProps.locationClicked ||
        !nextProps.locationClicked) &&
      this.props.location === nextProps.location
    ) {
      return;
    }

    let newPos = null;
    if (this.props.location !== nextProps.location) {
      newPos = {
        lat: nextProps.location.lat,
        lng: nextProps.location.lng,
        locationType: nextProps.location.locationType
      };
    }

    if (newPos === null) {
      newPos = {
        lat: nextProps.locationClicked.lat,
        lng: nextProps.locationClicked.lng,
        locationType: nextProps.locationClicked.locationType
      };
    }

    this.handleViewStateChange({
      viewState: Object.assign({}, this.state.viewState, {
        latitude: newPos.lat,
        longitude: newPos.lng,
        zoom: newPos.locationType === LocationTypes.HOME ? 17 : 18,
        transitionInterpolator: new FlyToInterpolator(),
        transitionDuration: 1200
      })
    });

    if (this.state.viewState.zoom < 15) {
      this.props.onClearLocation();
    }
  }

  handleViewStateChange({viewState}) {
    this.setState({viewState});

    if (!this.props.onViewportChange) {
      return;
    }

    // Deriving the bounds from the view state rather than asking the map for
    // them keeps this independent of when the underlying map instance is
    // ready, which used to require holding a ref to it.
    const [west, south, east, north] = new WebMercatorViewport(
      Object.assign({}, viewState, {
        width: this.props.width,
        height: this.props.height
      })
    ).getBounds();

    this.props.onViewportChange(
      {
        sw: {lat: south, lng: west},
        ne: {lat: north, lng: east}
      },
      viewState.zoom
    );
  }

  handleDragStart() {
    this.props.onClearLocation();
  }

  render() {
    let zoom = this.state.viewState.zoom;
    let tween = zoom - 12;

    let layers = [];

    if (this.props.location) {
      let offset = 0.0001;
      let location = {
        type: "Feature",
        geometry: {
          type: "LineString",
          coordinates: [
            [this.props.location.lng - offset, this.props.location.lat],
            [this.props.location.lng, this.props.location.lat + offset],
            [this.props.location.lng + offset, this.props.location.lat],
            [this.props.location.lng, this.props.location.lat - offset],
            [this.props.location.lng - offset, this.props.location.lat]
          ]
        },

        properties: {
          lineColor: [241, 196, 15, 255],
          fillColor: [241, 196, 15, 255],
          radius: 1
        }
      };

      layers.push(
        new GeoJsonLayer({
          id: "location-home-point-layer",
          data: [location],
          opacity: 0.7,
          stroked: true,
          extruded: true,
          filled: true,
          lineWidthMinPixels: 4,
          getFillColor: featureFillColor,
          getLineColor: featureLineColor,
          getPointRadius: featurePointRadius
        })
      );
    }

    if (this.props.locationClicked) {
      let location = {
        type: "Feature",
        geometry: {
          type: "Point",
          coordinates: [
            this.props.locationClicked.lng,

            this.props.locationClicked.lat
          ]
        },
        properties: {
          lineColor: [0, 0, 0, 255],
          fillColor: [0, 255, 0, 255],
          radius: 1
        }
      };

      layers.push(
        new GeoJsonLayer({
          id: "location-clicked-point-layer",
          data: [location],
          opacity: 0.4,
          stroked: true,
          filled: true,
          lineWidthMinPixels: 2,
          pointRadiusScale: 30,
          getFillColor: featureFillColor,
          getLineColor: featureLineColor,
          getPointRadius: featurePointRadius
        })
      );
    }

    if (this.props.stopsPointData) {
      layers.push(
        new GeoJsonLayer({
          id: "stops-point-layer",
          data: this.props.stopsPointData,
          opacity: 1 - clamp(tween),
          stroked: true,
          filled: true,
          pointRadiusScale:
            696.0864 - 106.8473 * zoom + 4.205566 * Math.pow(zoom, 2),
          visible: tween < 1,
          getFillColor: featureFillColor,
          getLineColor: featureLineColor,
          getPointRadius: featurePointRadius
        })
      );
    }

    if (this.props.stopsIconData) {
      layers.push(
        new IconLayer({
          id: "stops-icon-layer",
          data: this.props.stopsIconData,
          iconAtlas: "/assets/sprites.png",
          iconMapping: IconMapping,
          visible: tween > 0,
          opacity: 1,
          getPosition: d => d.position,
          getIcon: d => d.icon,
          getSize: d => d.size,
          sizeScale:
            -40.28287 + 0.1462691 * zoom + 0.3593278 * Math.pow(zoom, 2)
        })
      );
    }

    if (this.props.lineData) {
      layers = layers.concat(
        this.props.lineData.map((l, i) => {
          return new GeoJsonLayer({
            id: "geojson-line-layer" + i,
            data: l,
            getLineColor: () => l.color,
            lineWidthMinPixels: l.width
          });
        })
      );
    }

    if (this.props.vehiclesPointData) {
      layers.push(
        new GeoJsonLayer({
          id: "vehciles-point-layer",
          data: this.props.vehiclesPointData,
          opacity: 1 - clamp(tween),
          stroked: true,
          filled: true,
          pointRadiusScale:
            696.0864 - 106.8473 * zoom + 4.205566 * Math.pow(zoom, 2),
          visible: tween < 1,
          getFillColor: featureFillColor,
          getLineColor: featureLineColor,
          getPointRadius: featurePointRadius
        })
      );
    }

    if (this.props.vehiclesIconData) {
      layers.push(
        new IconLayer({
          id: "vehicle-icon-layer",
          data: this.props.vehiclesIconData,
          iconAtlas: "/assets/sprites.png",
          iconMapping: IconMapping,
          visible: tween > 0,
          opacity: 1,
          getPosition: d => d.position,
          getIcon: d => d.icon,
          getSize: d => d.size,
          sizeScale:
            -40.28287 + 0.1462691 * zoom + 0.3593278 * Math.pow(zoom, 2)
        })
      );
    }

    return (
      <div id="mapbox" className="app-map">
        <DeckGL
          width={this.props.width}
          height={this.props.height}
          viewState={this.state.viewState}
          onViewStateChange={this.handleViewStateChange}
          onDragStart={this.handleDragStart}
          controller={true}
          layers={layers}>
          <Map mapStyle={MAP_STYLE} />
        </DeckGL>
      </div>
    );
  }
}

MapBox.propTypes = {
  width: PropTypes.number.isRequired,
  height: PropTypes.number.isRequired,
  onViewportChange: PropTypes.func,
  onClearLocation: PropTypes.func,
  location: TrimetricPropTypes.location,
  locationClicked: TrimetricPropTypes.locationClicked,
  stopsPointData: PropTypes.array,
  stopsIconData: PropTypes.array,
  vehiclesPointData: PropTypes.array,
  vehiclesIconData: PropTypes.array,
  lineData: PropTypes.array
};

function mapDispatchToProps(dispatch) {
  return {
    onClearLocation: () => {
      dispatch(clearLocation());
    }
  };
}

function mapStateToProps(state) {
  return {
    location: state.location,
    locationClicked: state.locationClicked,
    stopsPointData: state.stopsPointData,
    stopsIconData: state.stopsIconData,
    vehiclesPointData: state.vehiclesPointData,
    vehiclesIconData: state.vehiclesIconData,
    lineData: state.lineData
  };
}

export default connect(mapStateToProps, mapDispatchToProps)(MapBox);
